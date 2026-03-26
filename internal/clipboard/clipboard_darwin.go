//go:build darwin

package clipboard

import (
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
		return c.readPlainText()
	}

	best := bestType(types)
	if best == "" {
		return c.readPlainText()
	}

	return c.readType(best)
}

func (c *darwinClipboard) Write(ct Content) error {
	uti, ok := mimeToUTI[ct.MimeType]
	if !ok {
		return fmt.Errorf("unsupported MIME type for macOS clipboard: %s", ct.MimeType)
	}

	// Write raw bytes to a temp file, then load via NSData to bypass
	// pbcopy's locale-dependent encoding (macOS Roman under launchd).
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

func (c *darwinClipboard) Clear() error {
	script := `
use framework "AppKit"
set pb to current application's NSPasteboard's generalPasteboard()
pb's clearContents()
`
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
		return c.readPlainText()
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

// readPlainText reads text from NSPasteboard as raw UTF-8 bytes via base64,
// bypassing both pbpaste (locale-dependent encoding) and osascript's text
// coercion (macOS Roman). Base64 is ASCII-safe so the encoding survives stdout.
func (c *darwinClipboard) readPlainText() (Content, error) {
	script := `
use framework "AppKit"
use framework "Foundation"
set pb to current application's NSPasteboard's generalPasteboard()
set textData to pb's dataForType:"public.utf8-plain-text"
if textData is not missing value then
	return (textData's base64EncodedStringWithOptions:0) as text
end if
return ""
`
	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		return Content{}, err
	}
	b64 := strings.TrimSpace(string(out))
	if b64 == "" {
		return Content{}, fmt.Errorf("no text data on clipboard")
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return Content{}, fmt.Errorf("decode clipboard text: %w", err)
	}
	return Content{MimeType: "text/plain", Data: data}, nil
}

func (c *darwinClipboard) readHTML(uti string) (Content, error) {
	script := fmt.Sprintf(`
use framework "AppKit"
use framework "Foundation"
set pb to current application's NSPasteboard's generalPasteboard()
set htmlData to pb's dataForType:"%s"
if htmlData is not missing value then
	return (htmlData's base64EncodedStringWithOptions:0) as text
end if
return ""
`, uti)
	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		return Content{}, err
	}
	b64 := strings.TrimSpace(string(out))
	if b64 == "" {
		return Content{}, fmt.Errorf("no HTML data on clipboard")
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return Content{}, fmt.Errorf("decode clipboard html: %w", err)
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
