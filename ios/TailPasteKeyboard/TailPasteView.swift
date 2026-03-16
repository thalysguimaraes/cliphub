import SwiftUI
import ClipHubKit

struct TailPasteView: View {
    @Bindable var viewModel: KeyboardViewModel

    var body: some View {
        VStack(spacing: 8) {
            // Main paste button.
            Button(action: viewModel.insertCurrent) {
                HStack {
                    Image(systemName: "doc.on.clipboard.fill")
                    if let clip = viewModel.currentClip, clip.isText {
                        Text(clip.preview)
                            .lineLimit(1)
                            .truncationMode(.tail)
                    } else {
                        Text("Tail Paste")
                    }
                }
                .font(.body.weight(.medium))
                .frame(maxWidth: .infinity)
                .padding(.vertical, 12)
            }
            .buttonStyle(.borderedProminent)
            .disabled(viewModel.currentClip == nil || viewModel.currentClip?.isText != true)

            // Recent clips strip.
            if !viewModel.recentClips.isEmpty {
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: 8) {
                        ForEach(viewModel.recentClips.prefix(5)) { clip in
                            Button(action: { viewModel.insert(clip) }) {
                                Text(clip.preview)
                                    .font(.caption)
                                    .lineLimit(1)
                                    .padding(.horizontal, 10)
                                    .padding(.vertical, 6)
                                    .background(.gray.opacity(0.15))
                                    .cornerRadius(8)
                            }
                            .buttonStyle(.plain)
                        }
                    }
                    .padding(.horizontal, 4)
                }
            }

            // Status line.
            HStack {
                if viewModel.isLoading {
                    ProgressView()
                        .controlSize(.mini)
                }
                if let error = viewModel.errorMessage {
                    Text(error)
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                }
                Spacer()
            }
            .padding(.horizontal, 4)
            .frame(height: 16)
        }
        .padding(8)
    }
}
