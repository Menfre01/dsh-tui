# Security Policy

## Reporting vulnerabilities

If you find a security vulnerability, **please do not** open a public issue.

Email menfre@proton.me instead; PGP encryption is preferred.

We will acknowledge receipt within 48 hours and credit you publicly after the
fix (mention it if you prefer to stay anonymous).

## Supported versions

| Version | Supported |
|---------|-----------|
| v0.0.x  | ✅ Security fixes |

## Security checklist

- dsh-tui is a pure client: it holds no API keys, stores no credentials — all model calls go through the host
- Connection trust is based on a loopback Host header — do not point `--url` at untrusted addresses
- Approvals/questions are user-confirmed: every sandbox-escalation request pops an approval overlay — don't blindly allow
- Keep session permissions minimal: default `workspace-write`; use `danger-full-access` only temporarily when needed
- Don't commit host addresses, session ids, or configuration to public repositories
