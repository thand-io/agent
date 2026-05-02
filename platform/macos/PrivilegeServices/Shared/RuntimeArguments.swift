import Foundation

public enum RuntimeArgumentError: Error, CustomStringConvertible, Equatable {
    case unsupportedArgument(String)
    case missingValue(String)
    case missingGRPCSocketPath

    public var description: String {
        switch self {
        case .unsupportedArgument(let value):
            return "unsupported argument \(value)"
        case .missingValue(let flag):
            return "\(flag) requires a value"
        case .missingGRPCSocketPath:
            return "missing required --grpc-socket-path argument"
        }
    }
}

public struct BrokerHelperArguments: Equatable {
    public var serviceLabel: String?
    public var grpcSocketPath: String?

    public init(serviceLabel: String? = nil, grpcSocketPath: String? = nil) {
        self.serviceLabel = serviceLabel
        self.grpcSocketPath = grpcSocketPath
    }

    public static func parse(_ arguments: [String]) throws -> BrokerHelperArguments {
        var parsed = BrokerHelperArguments()
        var index = 0

        while index < arguments.count {
            switch arguments[index] {
            case "serve":
                index += 1
            case "--service-label":
                index += 1
                guard index < arguments.count else {
                    throw RuntimeArgumentError.missingValue("--service-label")
                }
                parsed.serviceLabel = arguments[index]
                index += 1
            case "--grpc-socket-path":
                index += 1
                guard index < arguments.count else {
                    throw RuntimeArgumentError.missingValue("--grpc-socket-path")
                }
                parsed.grpcSocketPath = arguments[index]
                index += 1
            default:
                throw RuntimeArgumentError.unsupportedArgument(arguments[index])
            }
        }

        guard let grpcSocketPath = parsed.grpcSocketPath, !grpcSocketPath.isEmpty else {
            throw RuntimeArgumentError.missingGRPCSocketPath
        }
        return parsed
    }
}

public struct NotifierRuntimeArguments: Equatable {
    public var serviceLabel: String?

    public init(serviceLabel: String? = nil) {
        self.serviceLabel = serviceLabel
    }

    public static func parse(_ arguments: [String]) throws -> NotifierRuntimeArguments {
        var parsed = NotifierRuntimeArguments()
        var index = 0

        while index < arguments.count {
            switch arguments[index] {
            case "--service-label":
                index += 1
                guard index < arguments.count else {
                    throw RuntimeArgumentError.missingValue("--service-label")
                }
                parsed.serviceLabel = arguments[index]
                index += 1
            default:
                throw RuntimeArgumentError.unsupportedArgument(arguments[index])
            }
        }

        return parsed
    }
}
