import Foundation

public enum ClipHubError: Error, LocalizedError {
    case noHubURL
    case hubUnreachable(underlying: Error)
    case httpError(statusCode: Int, body: String)
    case emptyClipboard
    case decodingError(Error)

    public var errorDescription: String? {
        switch self {
        case .noHubURL: return "Hub URL not configured"
        case .hubUnreachable(let e): return "Hub unreachable: \(e.localizedDescription)"
        case .httpError(let code, let body): return "HTTP \(code): \(body)"
        case .emptyClipboard: return "Clipboard is empty on the hub"
        case .decodingError(let e): return "Decode error: \(e.localizedDescription)"
        }
    }
}

/// Async REST client for the ClipHub API.
public actor ClipHubClient {
    private let session: URLSession
    private let decoder: JSONDecoder
    public var baseURL: URL?
    public var sourceName: String

    public init(baseURL: URL? = nil, sourceName: String = "iphone") {
        self.baseURL = baseURL
        self.sourceName = sourceName

        let config = URLSessionConfiguration.default
        config.timeoutIntervalForRequest = 10
        config.timeoutIntervalForResource = 30
        config.waitsForConnectivity = true
        self.session = URLSession(configuration: config)

        self.decoder = JSONDecoder()
        self.decoder.dateDecodingStrategy = .iso8601
    }

    // MARK: - GET /api/clip

    public func getCurrentClip() async throws -> ClipItem? {
        let (data, response) = try await get("/api/clip")
        if response.statusCode == 204 { return nil }
        try validate(response, body: data)
        return try decoder.decode(ClipItem.self, from: data)
    }

    // MARK: - GET /api/clip/history

    public func getHistory(limit: Int = 50) async throws -> [ClipItem] {
        let (data, response) = try await get("/api/clip/history?limit=\(limit)")
        try validate(response, body: data)
        return try decoder.decode([ClipItem].self, from: data)
    }

    // MARK: - POST /api/clip (text)

    public func postClip(content: String, mimeType: String = "text/plain") async throws -> ClipItem {
        let body: [String: Any] = ["content": content, "mime_type": mimeType]
        return try await post("/api/clip", json: body)
    }

    // MARK: - POST /api/clip (binary)

    public func postClip(data: Data, mimeType: String) async throws -> ClipItem {
        let body: [String: Any] = ["data": data.base64EncodedString(), "mime_type": mimeType]
        return try await post("/api/clip", json: body)
    }

    // MARK: - GET /api/status

    public func getStatus() async throws -> HubStatus {
        let (data, response) = try await get("/api/status")
        try validate(response, body: data)
        return try decoder.decode(HubStatus.self, from: data)
    }

    // MARK: - Probe

    public func probe() async -> Bool {
        do {
            _ = try await getStatus()
            return true
        } catch {
            return false
        }
    }

    // MARK: - Helpers

    private func resolveURL(_ path: String) throws -> URL {
        guard let baseURL else { throw ClipHubError.noHubURL }
        return baseURL.appendingPathComponent(path)
    }

    private func get(_ path: String) async throws -> (Data, HTTPURLResponse) {
        let url = try resolveURL(path)
        return try await perform(URLRequest(url: url))
    }

    private func post(_ path: String, json body: [String: Any]) async throws -> ClipItem {
        let url = try resolveURL(path)
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.setValue(sourceName, forHTTPHeaderField: "X-Clip-Source")
        request.httpBody = try JSONSerialization.data(withJSONObject: body)

        let (data, response) = try await perform(request)
        try validate(response, body: data)
        return try decoder.decode(ClipItem.self, from: data)
    }

    private func perform(_ request: URLRequest) async throws -> (Data, HTTPURLResponse) {
        do {
            let (data, response) = try await session.data(for: request)
            guard let http = response as? HTTPURLResponse else {
                throw ClipHubError.hubUnreachable(underlying: URLError(.badServerResponse))
            }
            return (data, http)
        } catch let e as ClipHubError {
            throw e
        } catch {
            throw ClipHubError.hubUnreachable(underlying: error)
        }
    }

    private func validate(_ response: HTTPURLResponse, body: Data) throws {
        guard (200..<300).contains(response.statusCode) else {
            throw ClipHubError.httpError(
                statusCode: response.statusCode,
                body: String(data: body, encoding: .utf8) ?? ""
            )
        }
    }
}
