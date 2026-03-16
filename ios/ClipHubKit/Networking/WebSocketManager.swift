import Foundation

/// Manages a WebSocket connection to the hub's /api/clip/stream endpoint.
/// Reconnects with exponential backoff. Container app only.
@Observable
public final class WebSocketManager: @unchecked Sendable {
    private var task: URLSessionWebSocketTask?
    private let session: URLSession
    private var lastSeq: UInt64 = 0
    private var isRunning = false

    public var baseURL: URL?
    public var onUpdate: ((ClipItem) -> Void)?
    public var connectionState: ConnectionState = .disconnected(reason: "Not started")

    public init(session: URLSession = .shared) {
        self.session = session
    }

    public func start() {
        guard !isRunning else { return }
        isRunning = true
        Task { await connectLoop() }
    }

    public func stop() {
        isRunning = false
        task?.cancel(with: .goingAway, reason: nil)
        task = nil
        connectionState = .disconnected(reason: "Stopped")
    }

    private func connectLoop() async {
        var backoff: UInt64 = 1_000_000_000

        while isRunning {
            do {
                connectionState = .connecting
                try await connect()
                backoff = 1_000_000_000
            } catch {
                if !isRunning { return }
                connectionState = .disconnected(reason: error.localizedDescription)
                try? await Task.sleep(nanoseconds: backoff)
                backoff = min(backoff * 2, 60_000_000_000)
            }
        }
    }

    private func connect() async throws {
        guard let baseURL else { throw ClipHubError.noHubURL }

        var urlString = baseURL.appendingPathComponent("/api/clip/stream").absoluteString
        if urlString.hasPrefix("https") {
            urlString = "wss" + urlString.dropFirst(5)
        } else if urlString.hasPrefix("http") {
            urlString = "ws" + urlString.dropFirst(4)
        }
        if lastSeq > 0 {
            urlString += "?since_seq=\(lastSeq)"
        }

        guard let url = URL(string: urlString) else { return }
        let wsTask = session.webSocketTask(with: url)
        self.task = wsTask
        wsTask.resume()
        connectionState = .connected

        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601

        while isRunning {
            let message = try await wsTask.receive()
            let data: Data
            switch message {
            case .string(let text): data = Data(text.utf8)
            case .data(let d): data = d
            @unknown default: continue
            }

            guard let wsMsg = try? decoder.decode(WSMessage.self, from: data),
                  wsMsg.type == "clip_update",
                  let item = wsMsg.item else { continue }

            lastSeq = item.seq
            onUpdate?(item)
        }
    }
}
