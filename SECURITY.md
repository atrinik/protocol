# Security policy

Report suspected vulnerabilities privately through GitHub Security Advisories.
Do not include credentials, private identities, packet captures containing
player data, or exploit details in public issues.

Game Protocol 1 treats decoded Protobuf as untrusted. Consumers must enforce
semantic bounds, authorization, stream state, revisions, and transaction
atomicity before commit. Oversized declared frames fail before allocation;
invalid/truncated messages cannot partially mutate state. See
[`spec/transport.md`](spec/transport.md) for downgrade, identity pinning,
0-RTT, replay, amplification, queue, diagnostic-privacy, and close behavior.
