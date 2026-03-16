import UIKit
import SwiftUI

class ShareViewController: UIViewController {
    override func viewDidLoad() {
        super.viewDidLoad()

        let vm = ShareViewModel()
        let shareView = ShareView(viewModel: vm, onDismiss: { [weak self] in
            self?.extensionContext?.completeRequest(returningItems: nil)
        })

        let host = UIHostingController(rootView: shareView)
        addChild(host)
        host.view.frame = view.bounds
        host.view.autoresizingMask = [.flexibleWidth, .flexibleHeight]
        view.addSubview(host.view)
        host.didMove(toParent: self)

        // Extract input items.
        if let items = extensionContext?.inputItems as? [NSExtensionItem] {
            Task { await vm.extractContent(from: items) }
        }
    }
}
