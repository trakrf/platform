# Security Policy

## Supported Versions

We release patches for security vulnerabilities. Which versions are eligible for patches depends on the Business Source License terms.

| Version | Supported          |
| ------- | ------------------ |
| latest  | :white_check_mark: |
| < 1.0   | :x:                |

## Reporting a Vulnerability

**DO NOT** open public issues for security vulnerabilities.

To report a security vulnerability, please use ONE of the following:

1. **Email**: admin@trakrf.id
2. **Contact Form**: https://trakrf.id/contact (select "Security Issue")
3. **GitHub Security Advisories**: [Report a vulnerability](https://github.com/trakrf/platform/security/advisories/new) (preferred)

### What to Include

Please provide:
- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if available)

### Response Timeline

- **Acknowledgment**: Within 48 hours
- **Initial Assessment**: Within 5 business days
- **Resolution Timeline**: Depends on severity
    - Critical: 7-14 days
    - High: 30 days
    - Medium/Low: 90 days

### Disclosure Policy

We follow coordinated disclosure:
1. Reporter submits vulnerability
2. We validate and develop fix
3. We release patched version
4. We publicly disclose after users have time to update (typically 30 days)

## Security Best Practices

When deploying TrakRF Platform:
- Always use TLS for API endpoints
- Rotate JWT secrets regularly
- Use strong database passwords
- Keep dependencies updated
- Enable audit logging

## Recognition

We maintain a [Security Hall of Fame](https://trakrf.id/security/thanks) for responsible disclosure.