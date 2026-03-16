import UIKit
import SwiftUI
import ClipHubKit

class KeyboardViewController: UIInputViewController {
    private var hostingController: UIHostingController<TailPasteView>?

    override func viewDidLoad() {
        super.viewDidLoad()

        let vm = KeyboardViewModel(proxy: textDocumentProxy)
        let tailPasteView = TailPasteView(viewModel: vm)
        let host = UIHostingController(rootView: tailPasteView)
        host.view.translatesAutoresizingMaskIntoConstraints = false
        host.view.backgroundColor = .clear

        addChild(host)
        self.view.addSubview(host.view)
        host.didMove(toParent: self)

        NSLayoutConstraint.activate([
            host.view.leadingAnchor.constraint(equalTo: self.view.leadingAnchor),
            host.view.trailingAnchor.constraint(equalTo: self.view.trailingAnchor),
            host.view.topAnchor.constraint(equalTo: self.view.topAnchor),
            host.view.bottomAnchor.constraint(equalTo: self.view.bottomAnchor),
        ])

        self.hostingController = host
    }

    override func viewWillAppear(_ animated: Bool) {
        super.viewWillAppear(animated)
        hostingController?.rootView.viewModel.refresh()
    }
}
