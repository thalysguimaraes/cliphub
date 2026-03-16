import SwiftUI
import ClipHubKit

struct OnboardingView: View {
    @Environment(AppViewModel.self) private var viewModel
    @State private var hubURLText = "http://cliphub"
    @State private var sourceName = ""
    @State private var isProbing = false
    @State private var probeError: String?

    var body: some View {
        NavigationStack {
            VStack(spacing: 24) {
                Spacer()

                Image(systemName: "doc.on.clipboard.fill")
                    .font(.system(size: 64))
                    .foregroundStyle(.tint)

                Text("ClipHub")
                    .font(.largeTitle.bold())

                Text("Sync your clipboard across devices over Tailscale.")
                    .font(.body)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.center)
                    .padding(.horizontal)

                Spacer()

                VStack(alignment: .leading, spacing: 12) {
                    Text("Hub URL")
                        .font(.headline)
                    TextField("http://cliphub or http://100.x.x.x", text: $hubURLText)
                        .textFieldStyle(.roundedBorder)
                        .autocorrectionDisabled()
                        .textInputAutocapitalization(.never)
                        .keyboardType(.URL)

                    Text("Device Name")
                        .font(.headline)
                    TextField("iPhone", text: $sourceName)
                        .textFieldStyle(.roundedBorder)

                    if let error = probeError {
                        Text(error)
                            .font(.caption)
                            .foregroundStyle(.red)
                    }
                }
                .padding(.horizontal)

                Button(action: connect) {
                    if isProbing {
                        ProgressView()
                            .frame(maxWidth: .infinity)
                    } else {
                        Text("Connect")
                            .frame(maxWidth: .infinity)
                    }
                }
                .buttonStyle(.borderedProminent)
                .controlSize(.large)
                .disabled(isProbing)
                .padding(.horizontal)

                Spacer()
            }
            .navigationBarHidden(true)
            .onAppear {
                if sourceName.isEmpty {
                    sourceName = AppGroupStore().sourceName
                }
            }
        }
    }

    private func connect() {
        guard let url = URL(string: hubURLText), !hubURLText.isEmpty else {
            probeError = "Invalid URL"
            return
        }

        isProbing = true
        probeError = nil

        Task {
            let name = sourceName.isEmpty ? "iphone" : sourceName
            let ok = await viewModel.configureHub(url: url, sourceName: name)
            isProbing = false
            if !ok {
                probeError = "Could not reach hub. Is Tailscale VPN active?"
            }
        }
    }
}
