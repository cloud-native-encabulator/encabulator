# Contributing to Cloud Native Encabulator Microservices Demo

Thank you for your interest in contributing to the Cloud Native Encabulator Microservices Demo! This document provides guidelines and information for contributors.

## Code of Conduct

This project adheres to a code of conduct that promotes a welcoming and inclusive environment. By participating, you are expected to uphold this code.

## How to Contribute

### Reporting Issues

- Use the GitHub issue tracker to report bugs or request features
- Search existing issues before creating a new one
- Provide clear, detailed descriptions with steps to reproduce
- Include relevant system information and logs

### Development Process

1. **Fork the repository** and create a feature branch from `main`
2. **Make your changes** following the coding standards
3. **Add tests** for new functionality
4. **Update documentation** as needed
5. **Ensure all tests pass** and code is properly formatted
6. **Submit a pull request** with a clear description

### Developer Certificate of Origin (DCO)

This project requires all contributors to sign off on their commits using the Developer Certificate of Origin (DCO). This certifies that you have the right to submit your contribution under the project's license.

Add the `-s` flag to your git commit command:

```bash
git commit -s -m "Your commit message"
```

This adds a "Signed-off-by" line to your commit message:

```
Signed-off-by: Your Name <your.email@example.com>
```

## Development Setup

### Prerequisites

- Finch (container runtime)
- Kubernetes cluster (local or cloud)
- kubectl
- Go 1.21+
- Node.js 18+
- Python 3.11+
- Helm (optional)

### Local Development

1. **Clone the repository:**
   ```bash
   git clone https://github.com/your-org/cloud-native-encabulator-microservices.git
   cd cloud-native-encabulator-microservices
   ```

2. **Setup EKS environment:**
   ```bash
   # Ensure EKS cluster context is active
   aws eks update-kubeconfig --region <region> --name <cluster-name>
   
   # Setup environment
   ./scripts/setup/eks-setup.sh
   ```

3. **Deploy to EKS cluster:**
   ```bash
   ./scripts/deploy/deploy-eks.sh
   ```

### Service Development

Each service follows language-specific conventions:

#### Go Services
- Use Go modules for dependency management
- Follow standard Go project layout
- Include unit tests with `go test`
- Use `golangci-lint` for linting

#### Node.js Services
- Use npm for dependency management
- Follow standard Node.js project structure
- Include tests with Jest or similar
- Use ESLint and Prettier for code formatting

#### Python Services
- Use pip or poetry for dependency management
- Follow PEP 8 style guidelines
- Include tests with pytest
- Use black, flake8, and isort for formatting

### Testing

#### Unit Tests
Run tests for individual services:

```bash
# Go service
cd services/*/service-name
go test ./...

# Node.js service
cd services/*/service-name
npm test

# Python service
cd services/*/service-name
python -m pytest
```

#### Integration Tests
Run integration tests with Finch Compose:

```bash
finch compose -f finch-compose.test.yml up --abort-on-container-exit
```

#### End-to-End Tests
Deploy to a test Kubernetes cluster and run E2E tests:

```bash
./scripts/test/e2e-tests.sh
```

## Coding Standards

### General Guidelines

- Write clear, self-documenting code
- Include comprehensive tests
- Follow language-specific best practices
- Use meaningful commit messages
- Keep pull requests focused and atomic

### Documentation

- Update README files for service changes
- Include API documentation for new endpoints
- Add architecture diagrams for significant changes
- Update troubleshooting guides as needed

### Commit Messages

Use conventional commit format:

```
type(scope): description

[optional body]

[optional footer]
```

Types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`

Examples:
```
feat(api-gateway): add rate limiting middleware
fix(data-store): resolve connection pool leak
docs(readme): update deployment instructions
```

## Pull Request Process

1. **Create a feature branch** from `main`
2. **Make your changes** with appropriate tests
3. **Update documentation** as needed
4. **Ensure CI passes** (linting, tests, security scans)
5. **Request review** from maintainers
6. **Address feedback** and update as needed
7. **Squash commits** if requested
8. **Merge** after approval

### Pull Request Template

```markdown
## Description
Brief description of changes

## Type of Change
- [ ] Bug fix
- [ ] New feature
- [ ] Breaking change
- [ ] Documentation update

## Testing
- [ ] Unit tests pass
- [ ] Integration tests pass
- [ ] Manual testing completed

## Checklist
- [ ] Code follows style guidelines
- [ ] Self-review completed
- [ ] Documentation updated
- [ ] Tests added/updated
- [ ] DCO sign-off included
```

## Technology Exploration Branches

When contributing to technology exploration branches:

1. **Base changes** on the main branch
2. **Maintain compatibility** with core demonstration features
3. **Document differences** clearly
4. **Provide migration guides** between branches
5. **Test thoroughly** with the new technology stack

## Release Process

### Versioning

This project uses Semantic Versioning (SemVer):
- `MAJOR.MINOR.PATCH`
- Major: Breaking changes
- Minor: New features (backward compatible)
- Patch: Bug fixes (backward compatible)

### Release Workflow

1. **Create release branch** from `main`
2. **Update version numbers** and changelogs
3. **Test thoroughly** in staging environment
4. **Create release tag** and GitHub release
5. **Deploy to production** via CD pipeline

## Community

### Communication Channels

- **GitHub Issues**: Bug reports and feature requests
- **GitHub Discussions**: General questions and community discussion
- **Pull Requests**: Code review and collaboration

### Getting Help

- Check existing documentation and issues
- Ask questions in GitHub Discussions
- Reach out to maintainers for complex issues

## Recognition

Contributors are recognized in:
- GitHub contributor graphs
- Release notes for significant contributions
- Project documentation acknowledgments

## License

By contributing to this project, you agree that your contributions will be licensed under the Apache License 2.0.

---

Thank you for contributing to the Cloud Native Encabulator Microservices Demo! Your contributions help make this educational resource better for everyone.