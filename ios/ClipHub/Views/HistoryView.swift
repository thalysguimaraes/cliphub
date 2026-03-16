import SwiftUI

struct HistoryView: View {
    @Environment(AppViewModel.self) private var viewModel

    var body: some View {
        NavigationStack {
            List(viewModel.history) { item in
                HStack {
                    VStack(alignment: .leading, spacing: 4) {
                        Text(item.preview)
                            .font(.body)
                            .lineLimit(2)

                        HStack(spacing: 8) {
                            Text(item.mimeType)
                                .font(.caption2)
                                .padding(.horizontal, 6)
                                .padding(.vertical, 2)
                                .background(.gray.opacity(0.15))
                                .cornerRadius(4)

                            Text(item.source)
                                .font(.caption2)
                                .foregroundStyle(.secondary)

                            Text(item.createdAt, style: .relative)
                                .font(.caption2)
                                .foregroundStyle(.tertiary)
                        }
                    }

                    Spacer()

                    Text("#\(item.seq)")
                        .font(.caption.monospacedDigit())
                        .foregroundStyle(.tertiary)
                }
                .padding(.vertical, 4)
            }
            .navigationTitle("History")
            .refreshable { await viewModel.refresh() }
            .overlay {
                if viewModel.history.isEmpty {
                    ContentUnavailableView(
                        "No History",
                        systemImage: "clock",
                        description: Text("Clipboard items will appear here.")
                    )
                }
            }
        }
    }
}
