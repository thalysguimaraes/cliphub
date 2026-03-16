import Foundation

/// File-based cache for offline history, stored in the App Group container.
public struct ClipCache: Sendable {
    private let fileURL: URL

    public init() {
        let container = FileManager.default.containerURL(
            forSecurityApplicationGroupIdentifier: AppGroupStore.suiteName
        ) ?? FileManager.default.temporaryDirectory
        self.fileURL = container.appendingPathComponent("clip_history_cache.json")
    }

    public func save(_ items: [ClipItem]) {
        let e = JSONEncoder(); e.dateEncodingStrategy = .iso8601
        guard let data = try? e.encode(items) else { return }
        try? data.write(to: fileURL, options: .atomic)
    }

    public func load() -> [ClipItem] {
        guard let data = try? Data(contentsOf: fileURL) else { return [] }
        let d = JSONDecoder(); d.dateDecodingStrategy = .iso8601
        return (try? d.decode([ClipItem].self, from: data)) ?? []
    }
}
