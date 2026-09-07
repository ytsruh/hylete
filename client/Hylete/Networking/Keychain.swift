import Foundation
import Security

/// Tiny wrapper around the iOS Keychain for storing the JWT
/// issued by the Hylete server. One generic-password item per
/// service; the token is the only secret we store so the API
/// is intentionally narrow.
///
/// Access is constrained to `kSecAttrAccessibleAfterFirstUnlock`
/// so the token survives device reboots but is never readable
/// while the device is locked. That matches the behavior of
/// most banking / fitness apps and is the right trade-off for
/// an auth token.
public enum Keychain {
    /// The Keychain `kSecAttrService` string used for every
    /// item this app stores. A single service groups all of
    /// our items under one searchable namespace in the
    /// Keychain Access app.
    private static let service = "com.hyleteapp.hylete"

    public enum KeychainError: Error, LocalizedError {
        case unhandled(OSStatus)

        public var errorDescription: String? {
            switch self {
            case .unhandled(let status):
                return "Keychain error (\(status))."
            }
        }
    }

    /// Read the string for the given key, or nil if no item
    /// is stored. Returns nil (not an error) for "not found"
    /// so callers can treat the cold-start case uniformly.
    public static func string(for key: String) throws -> String? {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: key,
            kSecReturnData as String: true,
            kSecMatchLimit as String: kSecMatchLimitOne,
        ]
        var item: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &item)
        switch status {
        case errSecSuccess:
            guard let data = item as? Data, let str = String(data: data, encoding: .utf8) else {
                return nil
            }
            return str
        case errSecItemNotFound:
            return nil
        default:
            throw KeychainError.unhandled(status)
        }
    }

    /// Store or replace the string for the given key.
    public static func set(_ value: String, for key: String) throws {
        let data = Data(value.utf8)
        // Try update first so we replace in place rather than
        // leaving a stale item alongside the new one.
        let updateQuery: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: key,
        ]
        let updateAttributes: [String: Any] = [
            kSecValueData as String: data,
            kSecAttrAccessible as String: kSecAttrAccessibleAfterFirstUnlock,
        ]
        let updateStatus = SecItemUpdate(updateQuery as CFDictionary, updateAttributes as CFDictionary)
        if updateStatus == errSecSuccess {
            return
        }
        if updateStatus == errSecItemNotFound {
            var addQuery = updateQuery
            addQuery[kSecValueData as String] = data
            addQuery[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlock
            let addStatus = SecItemAdd(addQuery as CFDictionary, nil)
            if addStatus == errSecSuccess {
                return
            }
            throw KeychainError.unhandled(addStatus)
        }
        throw KeychainError.unhandled(updateStatus)
    }

    /// Remove the item for the given key, if any. Idempotent:
    /// a missing item is not an error.
    public static func remove(_ key: String) throws {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: key,
        ]
        let status = SecItemDelete(query as CFDictionary)
        if status == errSecSuccess || status == errSecItemNotFound {
            return
        }
        throw KeychainError.unhandled(status)
    }
}
