import Foundation
import UIKit

@Observable
final class KeyboardViewModel {
    private let store = AppGroupStore()
    private weak var proxy: UITextDocumentProxy?

    var currentClip: ClipItem?
    var recentClips: [ClipItem] = []
    var isLoading = false
    var errorMessage: String?

    init(proxy: UITextDocumentProxy) {
        self.proxy = proxy
        // Load cached data immediately (no network).
        self.currentClip = store.cachedCurrentClip
        self.recentClips = store.cachedRecentClips.filter { $0.isText }
    }

    func refresh() {
        guard let hubURL = store.hubURL else {
            errorMessage = "Open ClipHub to set up"
            return
        }

        isLoading = true
        Task {
            let client = ClipHubClient(baseURL: hubURL, sourceName: store.sourceName)
            do {
                if let clip = try await client.getCurrentClip() {
                    await MainActor.run {
                        self.currentClip = clip
                        self.store.cachedCurrentClip = clip
                    }
                }
                let history = try await client.getHistory(limit: 10)
                await MainActor.run {
                    self.recentClips = history.filter { $0.isText }
                    self.store.cachedRecentClips = history
                    self.isLoading = false
                    self.errorMessage = nil
                }
            } catch {
                await MainActor.run {
                    self.isLoading = false
                    // Keep cached data visible, just note the error.
                    self.errorMessage = "Offline"
                }
            }
        }
    }

    func insertCurrent() {
        guard let clip = currentClip, clip.isText, let content = clip.content else { return }
        proxy?.insertText(content)
    }

    func insert(_ clip: ClipItem) {
        guard clip.isText, let content = clip.content else { return }
        proxy?.insertText(content)
    }
}
