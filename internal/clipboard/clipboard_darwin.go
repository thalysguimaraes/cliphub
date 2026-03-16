//go:build darwin

package clipboard

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// UTI to MIME mappings.
var (
	utiToMIME = map[string]string{
		"public.html":             "text/html",
		"public.utf8-plain-text":  "text/plain",
		"public.utf16-plain-text": "text/plain",
		"public.png":              "image/png",
	}
	mimeToUTI = map[string]string{
		"text/html":  "public.html",
		"text/plain": "public.utf8-plain-text",
		"image/png":  "public.png",
	}
)

type darwinClipboard struct{}

// New returns a Clipboard for macOS.
func New() (Clipboard, error) {
	return &darwinClipboard{}, nil
}

func (c *darwinClipboard) ReadBest() (Content, error) {
	types, err := c.listTypes()
	if err != nil {
		// Fall back to pbpaste.
		out, err := exec.Command("pbpaste").Output()
		if err != nil {
			return Content{}, err
		}
		return Content{MimeType: "text/plain", Data: out}, nil
	}

	best := bestType(types)
	if best == "" {
		// Fall back to pbpaste.
		out, err := exec.Command("pbpaste").Output()
		if err != nil {
			return Content{}, err
		}
		return Content{MimeType: "text/plain", Data: out}, nil
	}

	return c.readType(best)
}

func (c *darwinClipboard) Write(ct Content) error {
	if ct.MimeType == "text/plain" {
		cmd := exec.Command("pbcopy")
		cmd.Stdin = bytes.NewReader(ct.Data)
		return cmd.Run()
	}

	uti, ok := mimeToUTI[ct.MimeType]
	if !ok {
		return fmt.Errorf("unsupported MIME type for macOS clipboard: %s", ct.MimeType)
	}

	// Write data to temp file, then use osascript to set clipboard.
	tmp, err := os.CreateTemp("", "cliphub-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(ct.Data); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()

	script := fmt.Sprintf(`
use framework "AppKit"
use framework "Foundation"
set fileData to (current application's NSData's dataWithContentsOfFile:"%s")
set pb to current application's NSPasteboard's generalPasteboard()
pb's clearContents()
pb's setData:fileData forType:"%s"
`, tmp.Name(), uti)

	return exec.Command("osascript", "-e", script).Run()
}

func (c *darwinClipboard) listTypes() ([]string, error) {
	script := `
use framework "AppKit"
set pb to current application's NSPasteboard's generalPasteboard()
set types to pb's types() as list
set output to ""
repeat with t in types
	set output to output & (t as text) & linefeed
end repeat
return output
`
	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		return nil, err
	}

	var mimeTypes []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if mime, ok := utiToMIME[line]; ok {
			mimeTypes = append(mimeTypes, mime)
		}
	}
	return mimeTypes, nil
}

func (c *darwinClipboard) readType(mimeType string) (Content, error) {
	if mimeType == "text/plain" {
		out, err := exec.Command("pbpaste").Output()
		if err != nil {
			return Content{}, err
		}
		return Content{MimeType: "text/plain", Data: out}, nil
	}

	uti, ok := mimeToUTI[mimeType]
	if !ok {
		return Content{}, fmt.Errorf("unsupported MIME type: %s", mimeType)
	}

	if mimeType == "text/html" {
		return c.readHTML(uti)
	}
	return c.readBinary(uti, mimeType)
}

func (c *darwinClipboard) readHTML(uti string) (Content, error) {
	script := fmt.Sprintf(`
use framework "AppKit"
use framework "Foundation"
set pb to current application's NSPasteboard's generalPasteboard()
set htmlData to pb's dataForType:"%s"
if htmlData is not missing value then
	set htmlString to (current application's NSString's alloc()'s initWithData:htmlData encoding:(current application's NSUTF8StringEncoding))
	return htmlString as text
end if
return ""
`, uti)
	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		return Content{}, err
	}
	data := bytes.TrimSpace(out)
	if len(data) == 0 {
		return Content{}, fmt.Errorf("no HTML data on clipboard")
	}
	return Content{MimeType: "text/html", Data: data}, nil
}

func (c *darwinClipboard) readBinary(uti, mimeType string) (Content, error) {
	// Read binary data as base64 via osascript.
	script := fmt.Sprintf(`
use framework "AppKit"
use framework "Foundation"
set pb to current application's NSPasteboard's generalPasteboard()
set imgData to pb's dataForType:"%s"
if imgData is not missing value then
	return (imgData's base64EncodedStringWithOptions:0) as text
end if
return ""
`, uti)
	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		return Content{}, err
	}
	b64 := strings.TrimSpace(string(out))
	if b64 == "" {
		return Content{}, fmt.Errorf("no %s data on clipboard", mimeType)
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return Content{}, fmt.Errorf("decode base64: %w", err)
	}
	return Content{MimeType: mimeType, Data: data}, nil
}
