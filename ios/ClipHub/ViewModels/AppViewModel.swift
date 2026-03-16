import Foundation
import Observation
import ClipHubKit

@Observable
final class AppViewModel {
    private let store = AppGroupStore()
    private let cache = ClipCache()
    private var client: ClipHubClient
    private var wsManager = WebSocketManager()

    var currentClip: ClipItem?
    var history: [ClipItem] = []
    var connectionState: ConnectionState = .disconnected(reason: "Not started")
    var showOnboarding: Bool
    var isLoading = false
    var errorMessage: String?

    init() {
        self.showOnboarding = !store.onboardingCompleted
        self.client = ClipHubClient(baseURL: store.hubURL, sourceName: store.sourceName)
        self.history = cache.load()
        self.currentClip = store.cachedCurrentClip

        if store.onboardingCompleted {
            setupWebSocket()
        }
    }

    // MARK: - Configuration

    func configureHub(url: URL, sourceName: String) async -> Bool {
        let testClient = ClipHubClient(baseURL: url, sourceName: sourceName)
        let reachable = await testClient.probe()
        if reachable {
            store.hubURL = url
            store.sourceName = sourceName
            store.onboardingCompleted = true
            await client.updateBaseURL(url)
            wsManager.baseURL = url
            showOnboarding = false
            await refresh()
            setupWebSocket()
        }
        return reachable
    }

    // MARK: - Data

    func refresh() async {
        isLoading = true
        defer { isLoading = false }

        do {
            async let clipTask = client.getCurrentClip()
            async let histTask = client.getHistory(limit: 50)

            let (clip, hist) = try await (clipTask, histTask)
            currentClip = clip
            history = hist
            errorMessage = nil

            store.cachedCurrentClip = clip
            store.cachedRecentClips = hist
            cache.save(hist)
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    func sendToHub(content: String, mimeType: String = "text/plain") async {
        do {
            let item = try await client.postClip(content: content, mimeType: mimeType)
            currentClip = item
            errorMessage = nil
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    // MARK: - WebSocket

    private func setupWebSocket() {
        wsManager.baseURL = store.hubURL
        wsManager.onUpdate = { [weak self] item in
            guard let self else { return }
            Task { @MainActor in
                self.currentClip = item
                if !self.history.contains(where: { $0.seq == item.seq }) {
                    self.history.insert(item, at: 0)
                }
                self.store.cachedCurrentClip = item
                self.store.cachedRecentClips = self.history
            }
        }
        wsManager.start()
    }
}

// Extension to allow updating the actor's URL from outside
extension ClipHubClient {
    public func updateBaseURL(_ url: URL) async {
        baseURL = url
    }
}
