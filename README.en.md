# @go/task

See [API reference](README.md).

## AES-GCM configuration compatibility

Released dependencies select crypto v1.5.9. Shared Symmetric.Decrypt/DecryptBytes authenticate `12-byte IV || ciphertext || 16-byte tag` first, then try the configured legacy IV. Existing APIs remain unchanged. Prefer EncryptBytesAutoIV for new data and upgrade readers before enabling writers. CBC/SM4 formats are unchanged.

### Realtime log integration (unreleased, 2026-11-12)

Explicit local go.mod replacements link this change without go.work. File logs default to paired .gz/.dict files, read using logv or built-in application log commands. Keep both files together; NoCompress retains plaintext. Release dependencies in order and remove development replacements before tagging. Published version claims remain unchanged.
