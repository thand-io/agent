import Foundation

public func brokerLog(_ message: String, fields: [String: String] = [:]) {
    var line = "[thand-privilege-broker] \(Date().ISO8601Format()) pid=\(ProcessInfo.processInfo.processIdentifier) \(message)"
    if !fields.isEmpty {
        let renderedFields = fields
            .keys
            .sorted()
            .map { key in
                "\(key)=\(fields[key] ?? "")"
            }
            .joined(separator: " ")
        line += " " + renderedFields
    }
    fputs(line + "\n", stderr)
}
