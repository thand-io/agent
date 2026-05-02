import CryptoKit
import Foundation

public final class LeaseStore {
    private let directoryURL: URL
    private let fileManager: FileManager
    private let encoder: JSONEncoder
    private let decoder: JSONDecoder

    private var leasesDirectoryURL: URL {
        directoryURL.appendingPathComponent("leases", isDirectory: true)
    }

    private var grantsDirectoryURL: URL {
        directoryURL.appendingPathComponent("grants", isDirectory: true)
    }

    public init(
        directoryURL: URL,
        fileManager: FileManager = .default,
        encoder: JSONEncoder = BrokerJSON.makeEncoder(),
        decoder: JSONDecoder = BrokerJSON.makeDecoder()
    ) {
        self.directoryURL = directoryURL
        self.fileManager = fileManager
        self.encoder = encoder
        self.decoder = decoder
    }

    public func prepareDirectory() throws {
        try fileManager.createDirectory(at: directoryURL, withIntermediateDirectories: true)
        try fileManager.createDirectory(at: leasesDirectoryURL, withIntermediateDirectories: true)
        try fileManager.createDirectory(at: grantsDirectoryURL, withIntermediateDirectories: true)
        try migrateLegacyLeaseFilesIfNeeded()
    }

    public func save(_ lease: LeaseRecord) throws {
        try prepareDirectory()
        let data = try encoder.encode(lease)
        try data.write(to: leaseURL(for: lease.brokerHandle), options: .atomic)
    }

    public func load(handle: String) throws -> LeaseRecord? {
        let url = leaseURL(for: handle)
        guard fileManager.fileExists(atPath: url.path) else {
            return nil
        }
        let data = try Data(contentsOf: url)
        return try decoder.decode(LeaseRecord.self, from: data)
    }

    public func loadAll() throws -> [LeaseRecord] {
        try prepareDirectory()
        return try leaseURLs().map { url in
            let data = try Data(contentsOf: url)
            return try decoder.decode(LeaseRecord.self, from: data)
        }
    }

    public func remove(handle: String) throws {
        let url = leaseURL(for: handle)
        guard fileManager.fileExists(atPath: url.path) else {
            return
        }
        try fileManager.removeItem(at: url)
    }

    public func saveGrant(_ record: GrantLedgerRecord) throws {
        try prepareDirectory()
        let data = try encoder.encode(record)
        try data.write(to: grantURL(for: record.grantID), options: .atomic)
    }

    public func loadGrant(grantID: String) throws -> GrantLedgerRecord? {
        let url = grantURL(for: grantID)
        guard fileManager.fileExists(atPath: url.path) else {
            return nil
        }
        let data = try Data(contentsOf: url)
        return try decoder.decode(GrantLedgerRecord.self, from: data)
    }

    public func loadGrant(brokerHandle: String) throws -> GrantLedgerRecord? {
        try prepareDirectory()
        for url in try grantURLs() {
            let data = try Data(contentsOf: url)
            let record = try decoder.decode(GrantLedgerRecord.self, from: data)
            if record.brokerHandle == brokerHandle {
                return record
            }
        }
        return nil
    }

    public func loadAllGrants() throws -> [GrantLedgerRecord] {
        try prepareDirectory()
        return try grantURLs().map { url in
            let data = try Data(contentsOf: url)
            return try decoder.decode(GrantLedgerRecord.self, from: data)
        }
    }

    public func removeGrant(grantID: String) throws {
        let url = grantURL(for: grantID)
        guard fileManager.fileExists(atPath: url.path) else {
            return
        }
        try fileManager.removeItem(at: url)
    }

    @discardableResult
    public func pruneGrantTombstones(olderThan cutoff: Date) throws -> [GrantLedgerRecord] {
        try prepareDirectory()

        var removed: [GrantLedgerRecord] = []
        for url in try grantURLs() {
            let data = try Data(contentsOf: url)
            let record = try decoder.decode(GrantLedgerRecord.self, from: data)
            guard record.state != .active, let terminalAt = record.terminalAt, terminalAt < cutoff else {
                continue
            }
            try fileManager.removeItem(at: url)
            removed.append(record)
        }
        return removed
    }

    public func grantURL(for grantID: String) -> URL {
        grantsDirectoryURL.appendingPathComponent("\(safeGrantFilename(grantID)).json")
    }

    public func leaseURL(for handle: String) -> URL {
        leasesDirectoryURL.appendingPathComponent("\(handle).json")
    }

    private func leaseURLs() throws -> [URL] {
        try fileManager.contentsOfDirectory(at: leasesDirectoryURL, includingPropertiesForKeys: nil)
            .filter { $0.pathExtension == "json" }
            .sorted { $0.lastPathComponent < $1.lastPathComponent }
    }

    private func grantURLs() throws -> [URL] {
        try fileManager.contentsOfDirectory(at: grantsDirectoryURL, includingPropertiesForKeys: nil)
            .filter { $0.pathExtension == "json" }
            .sorted { $0.lastPathComponent < $1.lastPathComponent }
    }

    private func migrateLegacyLeaseFilesIfNeeded() throws {
        let contents = try fileManager.contentsOfDirectory(at: directoryURL, includingPropertiesForKeys: nil)
        let legacyFiles = contents
            .filter { $0.pathExtension == "json" }
            .sorted { $0.lastPathComponent < $1.lastPathComponent }

        for legacyURL in legacyFiles {
            let data = try Data(contentsOf: legacyURL)
            guard let lease = try? decoder.decode(LeaseRecord.self, from: data) else {
                continue
            }
            let destinationURL = leaseURL(for: lease.brokerHandle)
            if !fileManager.fileExists(atPath: destinationURL.path) {
                try fileManager.moveItem(at: legacyURL, to: destinationURL)
            } else {
                try fileManager.removeItem(at: legacyURL)
            }
        }
    }

    private func safeGrantFilename(_ grantID: String) -> String {
        SHA256.hash(data: Data(grantID.utf8)).map { String(format: "%02x", $0) }.joined()
    }
}
