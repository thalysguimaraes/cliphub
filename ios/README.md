# ClipHub iOS

iOS client for ClipHub: container app + Tail Paste keyboard + share extension.

## Setup

### Generate Xcode project

Install [xcodegen](https://github.com/yonaskolb/XcodeGen) and run:

```bash
cd ios
xcodegen generate
open ClipHub.xcodeproj
```

### Manual Xcode setup (alternative)

1. Create a new iOS App project named "ClipHub"
2. Add a Framework target "ClipHubKit"
3. Add a Custom Keyboard Extension target "TailPasteKeyboard"
4. Add a Share Extension target "TailClipShare"
5. Enable App Groups (`group.com.thalys.cliphub`) on all 4 targets
6. Add all Swift source files to their respective targets
7. All 3 extension/app targets should embed ClipHubKit

### Configuration

1. Set your Apple Developer Team ID in project.yml or Xcode signing settings
2. Build and run on your iPhone
3. On first launch, enter your hub URL (`http://cliphub` if MagicDNS is enabled, or `http://100.x.x.x`)
4. Enable the keyboard: Settings → General → Keyboard → Keyboards → Add → ClipHub → Allow Full Access

## Architecture

- **ClipHubKit**: shared framework with REST client, WebSocket manager, models, storage
- **ClipHub**: container app showing current clip, history, settings
- **TailPasteKeyboard**: custom keyboard with "Tail Paste" button to insert the current hub clip
- **TailClipShare**: share extension to send content from any app to the hub

## Requirements

- iOS 17+
- Tailscale VPN active (to reach the hub on your tailnet)
- ClipHub hub running on your tailnet
