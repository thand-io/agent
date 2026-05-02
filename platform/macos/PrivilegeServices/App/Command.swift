import Foundation

public enum PrivilegeServicesCommand: String, CaseIterable {
    case register
    case unregister
    case status
    case openSettings = "open-settings"

    public static func parse(arguments: ArraySlice<String>) throws -> PrivilegeServicesCommand {
        guard let argument = arguments.first else {
            return .status
        }
        guard let command = PrivilegeServicesCommand(rawValue: argument) else {
            throw PrivilegeServicesCommandError.unsupportedCommand(argument)
        }
        return command
    }

    public static var usage: String {
        let commands = Self.allCases.map(\.rawValue).joined(separator: ", ")
        return "usage: ThandPrivilegeServices [\(commands)]"
    }
}

public enum PrivilegeServicesCommandError: Error, CustomStringConvertible {
    case unsupportedCommand(String)

    public var description: String {
        switch self {
        case .unsupportedCommand(let command):
            return "unsupported command \(command)"
        }
    }
}
