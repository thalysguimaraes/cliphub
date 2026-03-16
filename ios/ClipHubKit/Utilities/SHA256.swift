import CryptoKit
import Foundation

/// SHA-256 hashing that matches the Go hub's protocol.HashContent / HashBytes.
public enum ClipHash {
    public static func sha256Hex(_ data: Data) -> String {
        let digest = SHA256.hash(data: data)
        return digest.map { String(format: "%02x", $0) }.joined()
    }

    public static func sha256Hex(_ string: String) -> String {
        sha256Hex(Data(string.utf8))
    }
}
