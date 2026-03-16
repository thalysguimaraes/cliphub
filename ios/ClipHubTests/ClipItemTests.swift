import XCTest
@testable import ClipHubKit

final class ClipItemTests: XCTestCase {

    /// Verify JSON round-trip matches the Go hub's format exactly.
    func testJSONRoundTrip() throws {
        let json = """
        {
            "seq": 42,
            "mime_type": "text/plain",
            "content": "hello world",
            "hash": "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
            "source": "omarchy",
            "created_at": "2026-03-16T12:00:00Z",
            "expires_at": "2026-03-17T12:00:00Z"
        }
        """.data(using: .utf8)!

        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601

        let item = try decoder.decode(ClipItem.self, from: json)
        XCTAssertEqual(item.seq, 42)
        XCTAssertEqual(item.mimeType, "text/plain")
        XCTAssertEqual(item.content, "hello world")
        XCTAssertNil(item.data)
        XCTAssertEqual(item.source, "omarchy")
        XCTAssertTrue(item.isText)
        XCTAssertEqual(item.preview, "hello world")
    }

    func testBinaryItem() throws {
        // Go encodes []byte as base64 in JSON.
        let json = """
        {
            "seq": 1,
            "mime_type": "image/png",
            "data": "iVBORw0KGgo=",
            "hash": "abc123",
            "source": "mac",
            "created_at": "2026-03-16T12:00:00Z",
            "expires_at": "2026-03-17T12:00:00Z"
        }
        """.data(using: .utf8)!

        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601

        let item = try decoder.decode(ClipItem.self, from: json)
        XCTAssertEqual(item.mimeType, "image/png")
        XCTAssertFalse(item.isText)
        XCTAssertNotNil(item.data)
        XCTAssertNil(item.content)
        XCTAssertEqual(item.preview, "[image/png, 10 bytes]")
    }

    func testSHA256MatchesGo() {
        // Go's protocol.HashContent("hello") produces this hash.
        let expected = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
        let got = ClipHash.sha256Hex("hello")
        XCTAssertEqual(got, expected)
    }

    func testWSMessageDecode() throws {
        let json = """
        {
            "type": "clip_update",
            "item": {
                "seq": 5,
                "mime_type": "text/plain",
                "content": "test",
                "hash": "abc",
                "source": "node1",
                "created_at": "2026-03-16T12:00:00Z",
                "expires_at": "2026-03-17T12:00:00Z"
            }
        }
        """.data(using: .utf8)!

        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601

        let msg = try decoder.decode(WSMessage.self, from: json)
        XCTAssertEqual(msg.type, "clip_update")
        XCTAssertEqual(msg.item?.seq, 5)
        XCTAssertEqual(msg.item?.content, "test")
    }
}
