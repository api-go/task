# @go/task

See [API reference](README.md).

## AES-GCM configuration compatibility

Released dependencies select crypto v1.5.9. Shared Symmetric.Decrypt/DecryptBytes authenticate `12-byte IV || ciphertext || 16-byte tag` first, then try the configured legacy IV. Existing APIs remain unchanged. Prefer EncryptBytesAutoIV for new data and upgrade readers before enabling writers. CBC/SM4 formats are unchanged.

### Realtime log integration (unreleased, 2026-11-12)

Released in v1.5.17 with published dependencies and no development replacements. File logs use paired .gz/.dict files; read through logv or embedded commands. Keep both files together; NoCompress retains plaintext.


### Release v1.5.17 — 2026-11-12

Released GitHub dependencies; no local development replacements. Full tests and benchmarks validated with GOWORK=off.
