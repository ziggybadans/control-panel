import SwiftUI

// Menu bar companion for the server control panel: glanceable status,
// one click to the full web UI.
@main
struct PanelBarApp: App {
    @StateObject private var model = PanelModel()

    var body: some Scene {
        MenuBarExtra {
            PanelView()
                .environmentObject(model)
        } label: {
            MenuBarLabel()
                .environmentObject(model)
        }
        .menuBarExtraStyle(.window)
    }
}

struct MenuBarLabel: View {
    @EnvironmentObject var model: PanelModel

    var body: some View {
        // MenuBarExtra labels render Image + Text only.
        switch model.status {
        case .unreachable, .authNeeded:
            Image(systemName: "server.rack")
            Text("!")
        case .degraded:
            Image(systemName: "exclamationmark.triangle")
            if model.showCPUInBar, let s = model.summary {
                Text(String(format: "%.0f%%", s.cpu))
            }
        default:
            Image(systemName: "server.rack")
            if model.showCPUInBar, let s = model.summary {
                Text(String(format: "%.0f%%", s.cpu))
            }
        }
    }
}
