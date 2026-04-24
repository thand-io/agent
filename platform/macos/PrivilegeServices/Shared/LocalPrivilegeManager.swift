import Darwin
import Foundation

public struct UIDRange: Sendable, Equatable {
    public let min: Int
    public let max: Int
}

public struct ValidatedLocalUser: Sendable, Equatable {
    public let username: String
    public let uid: Int
}

public enum BrokerServiceError: Error, CustomStringConvertible {
    case invalidRequest(String)
    case invalidUsername(String)
    case deniedUsername(String)
    case unknownUser(String)
    case uidOutOfRange(String, Int)
    case invalidUIDRange(String)
    case missingLease(String)
    case visudoFailed(String)
    case grantIDConflict(String)
    case grantAlreadyCompleted(String, GrantLedgerState)
    case unavailable(String)

    public var description: String {
        switch self {
        case .invalidRequest(let message):
            return message
        case .invalidUsername(let username):
            return "local username \(username) is invalid"
        case .deniedUsername(let username):
            return "local username \(username) is denied for privileged access"
        case .unknownUser(let username):
            return "local username \(username) could not be resolved"
        case .uidOutOfRange(let username, let uid):
            return "local user \(username) with UID \(uid) is outside allowed UID ranges"
        case .invalidUIDRange(let rawValue):
            return "invalid UID range \(rawValue)"
        case .missingLease(let handle):
            return "no active lease exists for broker handle \(handle)"
        case .visudoFailed(let message):
            return message
        case .grantIDConflict(let grantID):
            return "grant id \(grantID) was already used for a different timed sudoers grant request"
        case .grantAlreadyCompleted(let grantID, let state):
            return "grant id \(grantID) already completed with state \(state.rawValue)"
        case .unavailable(let message):
            return message
        }
    }
}

public final class LocalAccountValidator {
    public typealias UserLookup = @Sendable (String) -> ValidatedLocalUser?

    private let lookupUser: UserLookup

    public init(lookupUser: @escaping UserLookup = LocalAccountValidator.defaultLookup) {
        self.lookupUser = lookupUser
    }

    public func validate(
        username: String,
        deniedUsernames: [String],
        allowedUIDRanges: [String]
    ) throws -> ValidatedLocalUser {
        let trimmedUsername = username.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmedUsername.isEmpty, !trimmedUsername.contains(where: \.isWhitespace) else {
            throw BrokerServiceError.invalidUsername(username)
        }

        let denied = Set(
            (["root", "daemon", "nobody"] + deniedUsernames)
                .map { $0.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() }
                .filter { !$0.isEmpty }
        )
        if denied.contains(trimmedUsername.lowercased()) {
            throw BrokerServiceError.deniedUsername(trimmedUsername)
        }

        guard let validatedUser = lookupUser(trimmedUsername) else {
            throw BrokerServiceError.unknownUser(trimmedUsername)
        }

        let ranges = try Self.parseUIDRanges(allowedUIDRanges)
        guard ranges.contains(where: { validatedUser.uid >= $0.min && validatedUser.uid <= $0.max }) else {
            throw BrokerServiceError.uidOutOfRange(validatedUser.username, validatedUser.uid)
        }

        return validatedUser
    }

    public static func parseUIDRanges(_ rawValues: [String]) throws -> [UIDRange] {
        let values = rawValues.isEmpty ? ["500-60000"] : rawValues
        return try values.map { rawValue in
            let value = rawValue.trimmingCharacters(in: .whitespacesAndNewlines)
            if value.contains("-") {
                let components = value.split(separator: "-", maxSplits: 1).map(String.init)
                guard components.count == 2,
                      let minValue = Int(components[0].trimmingCharacters(in: .whitespacesAndNewlines)),
                      let maxValue = Int(components[1].trimmingCharacters(in: .whitespacesAndNewlines)),
                      minValue <= maxValue else {
                    throw BrokerServiceError.invalidUIDRange(rawValue)
                }
                return UIDRange(min: minValue, max: maxValue)
            }

            guard let exactValue = Int(value) else {
                throw BrokerServiceError.invalidUIDRange(rawValue)
            }
            return UIDRange(min: exactValue, max: exactValue)
        }
    }

    public static func defaultLookup(username: String) -> ValidatedLocalUser? {
        guard let record = getpwnam(username) else {
            return nil
        }

        let resolvedUsername = String(cString: record.pointee.pw_name)
        return ValidatedLocalUser(username: resolvedUsername, uid: Int(record.pointee.pw_uid))
    }

    public static func defaultLookup(uid: uid_t) -> ValidatedLocalUser? {
        guard let record = getpwuid(uid) else {
            return nil
        }

        let resolvedUsername = String(cString: record.pointee.pw_name)
        return ValidatedLocalUser(username: resolvedUsername, uid: Int(record.pointee.pw_uid))
    }
}

public final class SudoersGrantManager {
    public typealias CommandRunner = @Sendable ([String]) throws -> Void

    private let sudoersDirectoryURL: URL
    private let visudoPath: String
    private let fileManager: FileManager
    private let runCommand: CommandRunner

    public init(
        sudoersDirectoryURL: URL,
        visudoPath: String,
        fileManager: FileManager = .default,
        runCommand: @escaping CommandRunner = SudoersGrantManager.defaultCommandRunner
    ) {
        self.sudoersDirectoryURL = sudoersDirectoryURL
        self.visudoPath = visudoPath
        self.fileManager = fileManager
        self.runCommand = runCommand
    }

    public func installTimedGrant(username: String, grantID: String, brokerHandle: String) throws -> URL {
        try fileManager.createDirectory(at: sudoersDirectoryURL, withIntermediateDirectories: true)

        let targetURL = fragmentURL(for: brokerHandle)
        let temporaryURL = sudoersDirectoryURL.appendingPathComponent(".thand-\(UUID().uuidString.lowercased()).tmp")
        let content = "# Managed by Thand\n\(username) ALL=(ALL:ALL) NOPASSWD: ALL\n"

        try content.write(to: temporaryURL, atomically: true, encoding: .utf8)
        try fileManager.setAttributes([.posixPermissions: 0o440], ofItemAtPath: temporaryURL.path)

        do {
            try runCommand([visudoPath, "-c", "-f", temporaryURL.path])
        } catch {
            try? fileManager.removeItem(at: temporaryURL)
            throw error
        }

        if fileManager.fileExists(atPath: targetURL.path) {
            try fileManager.removeItem(at: targetURL)
        }
        try fileManager.moveItem(at: temporaryURL, to: targetURL)
        try fileManager.setAttributes([.posixPermissions: 0o440], ofItemAtPath: targetURL.path)
        return targetURL
    }

    public func revoke(fragmentURL: URL) throws {
        guard fileManager.fileExists(atPath: fragmentURL.path) else {
            return
        }
        try fileManager.removeItem(at: fragmentURL)
    }

    public func fragmentURL(for brokerHandle: String) -> URL {
        let safeHandle = brokerHandle.lowercased().map { character -> Character in
            if character.isLetter || character.isNumber || character == "-" || character == "_" {
                return character
            }
            return "-"
        }
        return sudoersDirectoryURL.appendingPathComponent("thand-\(String(safeHandle))")
    }

    public static func defaultCommandRunner(arguments: [String]) throws {
        guard let executable = arguments.first else {
            throw BrokerServiceError.invalidRequest("command arguments are required")
        }

        let process = Process()
        process.executableURL = URL(fileURLWithPath: executable)
        process.arguments = Array(arguments.dropFirst())

        let pipe = Pipe()
        process.standardOutput = pipe
        process.standardError = pipe

        try process.run()
        process.waitUntilExit()

        if process.terminationStatus != 0 {
            let output = String(decoding: pipe.fileHandleForReading.readDataToEndOfFile(), as: UTF8.self)
            let message = output.trimmingCharacters(in: .whitespacesAndNewlines)
            throw BrokerServiceError.visudoFailed(message.isEmpty ? "visudo validation failed" : message)
        }
    }
}
