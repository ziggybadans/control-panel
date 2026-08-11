import AppKit
import Foundation
import ServiceManagement
import SwiftUI

// PanelModel owns settings, the polling loop, and authentication against
// the control panel API.
@MainActor
final class PanelModel: ObservableObject {
    enum Status: Equatable {
        case ok
        case degraded(String) // reachable but something needs attention
        case unreachable(String)
        case authNeeded
        case unconfigured
    }

    @Published var summary: Summary?
    @Published var status: Status = .unconfigured
    @Published var showSettings = false

    // Settings (UserDefaults; password lives in the Keychain).
    @Published var urlString: String {
        didSet { defaults.set(urlString, forKey: "panelURL") }
    }
    @Published var pollInterval: Double {
        didSet {
            defaults.set(pollInterval, forKey: "pollInterval")
            schedule()
        }
    }
    @Published var showCPUInBar: Bool {
        didSet { defaults.set(showCPUInBar, forKey: "showCPUInBar") }
    }

    private let defaults = UserDefaults.standard
    private var timer: Timer?
    private let session: URLSession

    init() {
        let cfg = URLSessionConfiguration.default
        cfg.timeoutIntervalForRequest = 5
        cfg.httpCookieStorage = HTTPCookieStorage.shared
        session = URLSession(configuration: cfg)

        urlString = defaults.string(forKey: "panelURL") ?? "http://localhost:9090"
        let interval = defaults.double(forKey: "pollInterval")
        pollInterval = interval >= 2 ? interval : 10
        showCPUInBar = defaults.object(forKey: "showCPUInBar") as? Bool ?? true

        if urlString.isEmpty {
            status = .unconfigured
            showSettings = true
        }
        schedule()
        Task { await refresh() }
    }

    private var baseURL: URL? {
        URL(string: urlString.trimmingCharacters(in: .whitespaces))
    }

    private func schedule() {
        timer?.invalidate()
        let t = Timer(timeInterval: max(pollInterval, 2), repeats: true) { _ in
            Task { @MainActor [weak self] in await self?.refresh() }
        }
        t.tolerance = 1
        RunLoop.main.add(t, forMode: .common)
        timer = t
    }

    func refresh() async {
        guard let base = baseURL, !urlString.isEmpty else {
            status = .unconfigured
            return
        }
        do {
            var (data, code) = try await get(base.appendingPathComponent("api/summary"))
            if code == 401 {
                let ok = await login()
                guard ok else {
                    status = .authNeeded
                    return
                }
                (data, code) = try await get(base.appendingPathComponent("api/summary"))
            }
            guard code == 200 else {
                status = .unreachable("HTTP \(code)")
                return
            }
            let s = try JSONDecoder().decode(Summary.self, from: data)
            summary = s
            status = deriveStatus(s)
        } catch {
            status = .unreachable(shortError(error))
        }
    }

    private func deriveStatus(_ s: Summary) -> Status {
        if s.disksUnhealthy > 0 {
            return .degraded("\(s.disksUnhealthy) disk(s) failing SMART")
        }
        if !s.servicesFailed.isEmpty {
            return .degraded("\(s.servicesFailed.count) service(s) failed")
        }
        if s.mc.contains(where: { $0.state == "crashed" }) {
            return .degraded("a Minecraft server crashed")
        }
        return .ok
    }

    private func get(_ url: URL) async throws -> (Data, Int) {
        let (data, resp) = try await session.data(from: url)
        return (data, (resp as? HTTPURLResponse)?.statusCode ?? 0)
    }

    // login exchanges the Keychain password for a session cookie.
    func login() async -> Bool {
        guard let base = baseURL else { return false }
        let password = Keychain.loadPassword()
        guard !password.isEmpty else { return false }
        var req = URLRequest(url: base.appendingPathComponent("api/auth/login"))
        req.httpMethod = "POST"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.setValue("1", forHTTPHeaderField: "X-CP")
        req.httpBody = try? JSONEncoder().encode(["password": password])
        guard let (_, resp) = try? await session.data(for: req) else { return false }
        return (resp as? HTTPURLResponse)?.statusCode == 200
    }

    /// Settings "Save & test": returns nil on success, else an error string.
    func testConnection() async -> String? {
        guard baseURL != nil else { return "invalid URL" }
        await refresh()
        switch status {
        case .ok, .degraded: return nil
        case .authNeeded: return "authentication failed — check the password"
        case .unreachable(let e): return e
        case .unconfigured: return "no URL configured"
        }
    }

    func openPanel() {
        if let base = baseURL {
            NSWorkspace.shared.open(base)
        }
    }

    // MARK: launch at login

    var launchAtLogin: Bool {
        SMAppService.mainApp.status == .enabled
    }

    func setLaunchAtLogin(_ enable: Bool) {
        do {
            if enable {
                try SMAppService.mainApp.register()
            } else {
                try SMAppService.mainApp.unregister()
            }
        } catch {
            // Surface quietly; not critical.
            NSLog("launch-at-login: \(error)")
        }
        objectWillChange.send()
    }

    private func shortError(_ error: Error) -> String {
        let ns = error as NSError
        switch ns.code {
        case NSURLErrorCannotConnectToHost, NSURLErrorCannotFindHost:
            return "can't reach the panel"
        case NSURLErrorTimedOut:
            return "connection timed out"
        case NSURLErrorNotConnectedToInternet:
            return "offline"
        default:
            return ns.localizedDescription
        }
    }
}
