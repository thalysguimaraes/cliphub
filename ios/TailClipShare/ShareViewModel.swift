import Foundation
import UniformTypeIdentifiers
import ClipHubKit

@Observable
final class ShareViewModel {
    private let store = AppGroupStore()

    var previewText = "Loading..."
    var mimeType = "text/plain"
    var contentToSend: String?
    var dataToSend: Data?
    var isSending = false
    var didSend = false
    var errorMessage: String?

    func extractContent(from items: [NSExtensionItem]) async {
        for item in items {
            guard let providers = item.attachments else { continue }
            for provider in providers {
                // Text
                if provider.hasItemConformingToTypeIdentifier(UTType.plainText.identifier) {
                    if let text = try? await provider.loadItem(forTypeIdentifier: UTType.plainText.identifier) as? String {
                        contentToSend = text
                        mimeType = "text/plain"
                        previewText = String(text.prefix(200))
                        return
                    }
                }
                // URL
                if provider.hasItemConformingToTypeIdentifier(UTType.url.identifier) {
                    if let url = try? await provider.loadItem(forTypeIdentifier: UTType.url.identifier) as? URL {
                        contentToSend = url.absoluteString
                        mimeType = "text/plain"
                        previewText = url.absoluteString
                        return
                    }
                }
                // Image
                if provider.hasItemConformingToTypeIdentifier(UTType.image.identifier) {
                    if let data = try? await provider.loadDataRepresentation(for: .png) {
                        dataToSend = data
                        mimeType = "image/png"
                        previewText = "[Image, \(data.count) bytes]"
                        return
                    }
                }
            }
        }
        previewText = "No shareable content found"
    }

    func send() async {
        guard let hubURL = store.hubURL else {
            errorMessage = "Open ClipHub to configure"
            return
        }

        isSending = true
        defer { isSending = false }

        let client = ClipHubClient(baseURL: hubURL, sourceName: store.sourceName)

        do {
            if let content = contentToSend {
                _ = try await client.postClip(content: content, mimeType: mimeType)
            } else if let data = dataToSend {
                _ = try await client.postClip(data: data, mimeType: mimeType)
            } else {
                errorMessage = "Nothing to send"
                return
            }
            didSend = true
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}

// Helper for loading PNG data from NSItemProvider.
extension NSItemProvider {
    func loadDataRepresentation(for type: UTType) async throws -> Data {
        try await withCheckedThrowingContinuation { continuation in
            _ = loadDataRepresentation(forTypeIdentifier: type.identifier) { data, error in
                if let data {
                    continuation.resume(returning: data)
                } else {
                    continuation.resume(throwing: error ?? ClipHubError.emptyClipboard)
                }
            }
        }
    }
}
