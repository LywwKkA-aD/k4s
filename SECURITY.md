# Security Policy

  ## Supported Versions

  The following versions of k4s are currently being supported with security updates.

  | Version | Supported          |
  | ------- | ------------------ |
  | 0.2.x   | :white_check_mark: |
  | < 0.2   | :x:                |

  ## Reporting a Vulnerability

  We take security vulnerabilities seriously. If you discover a security issue in k4s, please report it responsibly.

  **Please do NOT report security vulnerabilities through public GitHub issues.**

  Instead, please report them via one of the following methods:

  1. **GitHub Private Vulnerability Reporting**: Use the [Security Advisories](https://github.com/LywwKkA-aD/k4s/security/advisories/new) feature
  2. **Email**: Contact the maintainer directly (if you have a contact email, add it here)

  ### What to include

  - Description of the vulnerability
  - Steps to reproduce the issue
  - Potential impact
  - Suggested fix (if any)

  ### What to expect

  - **Acknowledgment**: Within 48 hours of your report
  - **Status update**: Within 7 days with an assessment
  - **Resolution timeline**: Dependent on severity, typically within 30 days for critical issues

  ### Scope

  Security issues in the following areas are in scope:
  - Kubernetes credential handling
  - SSH key/passphrase management
  - Configuration file security
  - Command injection vulnerabilities
  - Unintended cluster modifications

  Thank you for helping keep k4s secure.

  This policy is appropriate for an early-stage project (v0.2.0) and covers the security-sensitive aspects of k4s (kubeconfig handling, SSH credentials, cluster operations).
