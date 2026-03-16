import SwiftUI
import ClipHubKit

struct SettingsView: View {
    @Environment(AppViewModel.self) private var viewModel
    private let store = AppGroupStore()

    @State private var hubURLText = ""
    @State private var sourceName = ""

    var body: some View {
        NavigationStack {
            Form {
                Section("Connection") {
                    LabeledContent("Status") {
                        HStack(spacing: 6) {
                            Circle()
                                .fill(statusColor)
                                .frame(width: 8, height: 8)
                            Text(viewModel.connectionState.label)
                                .font(.caption)
                        }
                    }

                    LabeledContent("Hub URL") {
                        Text(store.hubURL?.absoluteString ?? "Not set")
                            .font(.caption.monospaced())
                            .foregroundStyle(.secondary)
                    }

                    LabeledContent("Device Name") {
                        Text(store.sourceName)
                            .foregroundStyle(.secondary)
                    }
                }

                Section("Keyboard Extension") {
                    Text("Go to Settings → General → Keyboard → Keyboards → Add New Keyboard → ClipHub to enable Tail Paste.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }

                Section("About") {
                    LabeledContent("Version", value: Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "1.0")
                }

                Section {
                    Button("Reconfigure Hub", role: .destructive) {
                        viewModel.showOnboarding = true
                    }
                }
            }
            .navigationTitle("Settings")
        }
    }

    private var statusColor: Color {
        switch viewModel.connectionState {
        case .connected: return .green
        case .connecting: return .yellow
        case .disconnected: return .orange
        case .noVPN: return .red
        }
    }
}
