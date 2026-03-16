import SwiftUI

struct ShareView: View {
    @Bindable var viewModel: ShareViewModel
    let onDismiss: () -> Void

    var body: some View {
        NavigationStack {
            VStack(spacing: 20) {
                if viewModel.didSend {
                    Image(systemName: "checkmark.circle.fill")
                        .font(.system(size: 60))
                        .foregroundStyle(.green)
                    Text("Sent to Tail Clipboard")
                        .font(.headline)
                } else {
                    Text(viewModel.previewText)
                        .font(.body)
                        .lineLimit(5)
                        .padding()
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .background(.gray.opacity(0.1))
                        .cornerRadius(12)

                    Text(viewModel.mimeType)
                        .font(.caption)
                        .foregroundStyle(.secondary)

                    if let error = viewModel.errorMessage {
                        Text(error)
                            .font(.caption)
                            .foregroundStyle(.red)
                    }

                    Button(action: { Task { await viewModel.send() } }) {
                        if viewModel.isSending {
                            ProgressView()
                        } else {
                            Label("Send to Tail Clipboard", systemImage: "paperplane.fill")
                        }
                    }
                    .buttonStyle(.borderedProminent)
                    .disabled(viewModel.isSending)
                }
            }
            .padding()
            .navigationTitle("ClipHub")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel", action: onDismiss)
                }
                if viewModel.didSend {
                    ToolbarItem(placement: .confirmationAction) {
                        Button("Done", action: onDismiss)
                    }
                }
            }
        }
    }
}
