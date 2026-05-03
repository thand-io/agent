import Foundation
import PrivilegeServicesAppSupport

private let encoder: JSONEncoder = {
    let encoder = JSONEncoder()
    encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
    return encoder
}()

do {
    let command = try PrivilegeServicesCommand.parse(arguments: CommandLine.arguments.dropFirst())
    let controller = ServiceManagementController()

    switch command {
    case .register:
        try controller.register()
        FileHandle.standardOutput.write(Data("registered privilege services\n".utf8))
    case .unregister:
        try controller.unregister()
        FileHandle.standardOutput.write(Data("unregistered privilege services\n".utf8))
    case .status:
        let data = try encoder.encode(controller.currentStatus())
        FileHandle.standardOutput.write(data)
        FileHandle.standardOutput.write(Data("\n".utf8))
    case .openSettings:
        controller.openSettings()
    }
} catch let error as PrivilegeServicesCommandError {
    fputs("\(error)\n\(PrivilegeServicesCommand.usage)\n", stderr)
    exit(64)
} catch {
    fputs("ThandPrivilegeServices failed: \(error)\n", stderr)
    exit(1)
}
