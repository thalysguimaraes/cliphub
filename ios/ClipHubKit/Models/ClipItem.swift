import Foundation

/// Mirrors the hub's protocol.ClipItem JSON structure.
public struct ClipItem: Codable, Identifiable, Hashable, Sendable {
    public let seq: UInt64
    public let mimeType: String
    public let content: String?
    public let data: Data?
    public let hash: String
    public let source: String
    public let createdAt: Date
    public let expiresAt: Date

    public var id: UInt64 { seq }
    public var isText: Bool { mimeType.hasPrefix("text/") }

    public var rawBytes: Data {
        if isText, let content {
            return Data(content.utf8)
        }
        return data ?? Data()
    }

    public var preview: String {
        if isText, let content {
            let trimmed = content.prefix(200).replacingOccurrences(of: "\n", with: " ")
            return String(trimmed)
        }
        return "[\(mimeType), \(rawBytes.count) bytes]"
    }

    enum CodingKeys: String, CodingKey {
        case seq, content, data, hash, source
        case mimeType = "mime_type"
        case createdAt = "created_at"
        case expiresAt = "expires_at"
    }
}
