import Foundation
import PrivilegeServicesShared

do {
    let config = BrokerConfig.fromEnvironment()
    brokerLog("starting broker daemon", fields: [
        "service_label": config.serviceLabel,
        "state_dir": config.stateDirectoryURL.path,
        "sudoers_dir": config.sudoersDirectoryURL.path
    ])
    let service = try PrivilegeBrokerService(config: config)
    let controlServer = XPCBrokerServer(service: service)
    try controlServer.start()
    brokerLog("broker daemon activated", fields: [
        "service_label": config.serviceLabel
    ])
    RunLoop.main.run()
} catch {
    fputs("thand-macos-privilege-brokerd failed: \(error)\n", stderr)
    exit(1)
}
