import Foundation

public enum ConnectionState: Sendable, Equatable {
    case connected
    case connecting
    case disconnected(reason: String)
    case noVPN

    public var label: String {
        switch self {
        case .connected: return "Connected"
        case .connecting: return "Connecting…"
        case .disconnected(let reason): return "Disconnected: \(reason)"
        case .noVPN: return "VPN Required"
        }
    }

    public var isConnected: Bool {
        if case .connected = self { return true }
        return false
    }
}
