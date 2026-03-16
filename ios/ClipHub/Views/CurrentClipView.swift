import SwiftUI
import UniformTypeIdentifiers

struct CurrentClipView: View {
    @Environment(AppViewModel.self) private var viewModel

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: 16) {
                    if let clip = viewModel.currentClip {
                        clipContent(clip)
                    } else {
                        ContentUnavailableView(
                            "No Clipboard Content",
                            systemImage: "doc.on.clipboard",
                            description: Text("Copy something on another device to see it here.")
                        )
                    }
                }
                .padding()
            }
            .navigationTitle("Current Clip")
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button(action: { Task { await viewModel.refresh() } }) {
                        Image(systemName: "arrow.clockwise")
                    }
                }
            }
            .refreshable { await viewModel.refresh() }
        }
    }

    @ViewBuilder
    private func clipContent(_ clip: ClipItem) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Label(clip.mimeType, systemImage: clip.isText ? "doc.text" : "photo")
                    .font(.caption)
                    .foregroundStyle(.secondary)

                Spacer()

                Text("from \(clip.source)")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }

            if clip.isText, let content = clip.content {
                Text(content)
                    .font(.body.monospaced())
                    .textSelection(.enabled)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding()
                    .background(.gray.opacity(0.08))
                    .cornerRadius(12)
            } else if clip.mimeType == "image/png", let data = clip.data,
                      let uiImage = UIImage(data: data) {
                Image(uiImage: uiImage)
                    .resizable()
                    .aspectRatio(contentMode: .fit)
                    .cornerRadius(12)
            } else {
                Text("[\(clip.mimeType), \(clip.rawBytes.count) bytes]")
                    .foregroundStyle(.secondary)
            }

            HStack(spacing: 12) {
                Button(action: { copyToClipboard(clip) }) {
                    Label("Copy to iPhone", systemImage: "doc.on.doc")
                }
                .buttonStyle(.bordered)
            }
        }
    }

    private func copyToClipboard(_ clip: ClipItem) {
        if clip.isText, let content = clip.content {
            UIPasteboard.general.string = content
        } else if clip.mimeType == "image/png", let data = clip.data {
            UIPasteboard.general.setData(data, forPasteboardType: UTType.png.identifier)
        }
    }
}
