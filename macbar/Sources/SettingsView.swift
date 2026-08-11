import SwiftUI

struct SettingsView: View {
    @EnvironmentObject var model: PanelModel
    @State private var password = Keychain.loadPassword()
    @State private var testResult: String?
    @State private var testing = false
    @State private var launchAtLogin = false

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("Settings").font(.system(size: 13, weight: .semibold))

            VStack(alignment: .leading, spacing: 3) {
                Text("Panel URL").font(.system(size: 11)).foregroundStyle(.secondary)
                TextField("http://server:9090", text: $model.urlString)
                    .textFieldStyle(.roundedBorder)
                    .font(.system(size: 12).monospaced())
            }

            VStack(alignment: .leading, spacing: 3) {
                Text("Password").font(.system(size: 11)).foregroundStyle(.secondary)
                SecureField("panel password", text: $password)
                    .textFieldStyle(.roundedBorder)
                Text("Stored in the macOS Keychain.")
                    .font(.system(size: 10)).foregroundStyle(.tertiary)
            }

            VStack(alignment: .leading, spacing: 3) {
                Text("Refresh every").font(.system(size: 11)).foregroundStyle(.secondary)
                Picker("", selection: $model.pollInterval) {
                    Text("5 s").tag(5.0)
                    Text("10 s").tag(10.0)
                    Text("30 s").tag(30.0)
                    Text("60 s").tag(60.0)
                }
                .pickerStyle(.segmented)
                .labelsHidden()
            }

            Toggle("Show CPU % in the menu bar", isOn: $model.showCPUInBar)
                .font(.system(size: 12))
            Toggle("Launch at login", isOn: $launchAtLogin)
                .font(.system(size: 12))
                .onChange(of: launchAtLogin) { v in
                    model.setLaunchAtLogin(v)
                }

            if let result = testResult {
                Label(result == "ok" ? "Connected" : result,
                      systemImage: result == "ok" ? "checkmark.circle.fill" : "xmark.circle.fill")
                    .font(.system(size: 11))
                    .foregroundStyle(result == "ok" ? Color.green : Color.red)
            }

            HStack {
                Button("Back") {
                    model.showSettings = false
                }
                Spacer()
                Button {
                    save()
                } label: {
                    if testing {
                        ProgressView().controlSize(.small)
                    } else {
                        Text("Save & test")
                    }
                }
                .keyboardShortcut(.defaultAction)
                .disabled(testing)
            }
            .controlSize(.small)
        }
        .padding(12)
        .onAppear {
            launchAtLogin = model.launchAtLogin
        }
    }

    private func save() {
        Keychain.savePassword(password)
        testing = true
        testResult = nil
        Task {
            let err = await model.testConnection()
            testing = false
            testResult = err ?? "ok"
            if err == nil {
                try? await Task.sleep(nanoseconds: 700_000_000)
                model.showSettings = false
            }
        }
    }
}
