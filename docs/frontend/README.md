# TrakRF Handheld Documentation

Welcome to the TrakRF Handheld documentation. This guide helps you navigate the project's documentation structure and find the information you need.

## 📖 Documentation Organization

### Core Documentation

These documents are actively maintained and cover current development:

#### System Architecture
- **[`ARCHITECTURE.md`](./ARCHITECTURE.md)**: Complete system architecture overview
  - Module-based design patterns
  - State management with Zustand
  - Transport abstraction layer
  - Data flow and component interaction


#### Development Guides
- **[`MOCK_USAGE_GUIDE.md`](./MOCK_USAGE_GUIDE.md)**: BLE mock development setup and testing with simulateNotification
- **[`TEST-COMMANDS.md`](./TEST-COMMANDS.md)**: Test command reference and runner behavior
- **[`../prp/README.md`](../prp/README.md)**: Product Requirements Prompt (PRP) framework and workflow

#### CS108 Protocol Documentation
- **[`cs108/`](./cs108/)**: Protocol-specific documentation
  - **[`cs108/README.md`](./cs108/README.md)**: Protocol overview and quick reference
  - **[`cs108/CS108_and_CS463_Bluetooth_and_USB_Byte_Stream_API_Specifications.md`](./cs108/CS108_and_CS463_Bluetooth_and_USB_Byte_Stream_API_Specifications.md)**: Complete vendor specification
  - **[`cs108/inventory-parsing.md`](./cs108/inventory-parsing.md)**: Tag parsing quick reference
  - **[`CS108-PROTOCOL-QUIRKS.md`](./CS108-PROTOCOL-QUIRKS.md)**: Known protocol issues and workarounds

#### Deployment & Operations
- **[`DEPLOYMENT.md`](./DEPLOYMENT.md)**: Railway deployment guide with configuration and troubleshooting
- **[`OPENREPLAY-TROUBLESHOOTING.md`](./OPENREPLAY-TROUBLESHOOTING.md)**: Session replay debugging

#### Feature Documentation

## 🚀 Quick Start Guides

### For Developers
1. **Getting Started**: Read the main [`../README.md`](../README.md) for setup instructions
2. **Understanding the System**: Review [`ARCHITECTURE.md`](./ARCHITECTURE.md) for system overview
3. **Protocol Work**: Start with [`cs108/README.md`](./cs108/README.md) for CS108 specifics
4. **Development Guidelines**: Check [`../CLAUDE.md`](../CLAUDE.md) for coding conventions

### For Protocol Development
1. **CS108 Overview**: [`cs108/README.md`](./cs108/README.md)
2. **Tag Parsing**: [`cs108/inventory-parsing.md`](./cs108/inventory-parsing.md)
3. **Known Issues**: [`CS108-PROTOCOL-QUIRKS.md`](./CS108-PROTOCOL-QUIRKS.md)

### For Testing
1. **Mock Development & E2E Testing**: [`MOCK_USAGE_GUIDE.md`](./MOCK_USAGE_GUIDE.md)

## 📚 Documentation Types

### Technical Specifications
- Complete API references
- Protocol documentation
- Architecture patterns

### Implementation Guides
- Step-by-step implementation instructions
- Code examples and patterns
- Best practices and conventions

### Operational Documentation
- Deployment procedures
- Troubleshooting guides
- Performance monitoring

## 🔍 Finding Information

### By Topic
- **Architecture**: [`ARCHITECTURE.md`](./ARCHITECTURE.md)
- **Testing**: [`MOCK_USAGE_GUIDE.md`](./MOCK_USAGE_GUIDE.md)
- **Tag Parsing**: [`cs108/inventory-parsing.md`](./cs108/inventory-parsing.md)
- **Deployment**: [`DEPLOYMENT.md`](./DEPLOYMENT.md)

### By Role
- **New Developer**: Start with [`../README.md`](../README.md) → [`ARCHITECTURE.md`](./ARCHITECTURE.md)
- **Protocol Developer**: [`cs108/README.md`](./cs108/README.md) → Vendor specs PDFs
- **Test Engineer**: [`MOCK_USAGE_GUIDE.md`](./MOCK_USAGE_GUIDE.md)
- **DevOps**: [`DEPLOYMENT.md`](./DEPLOYMENT.md)

## 📝 Documentation Standards

- **Clear Navigation**: Each document includes relevant cross-references
- **Complete Context**: All necessary context included or linked
- **Consistent Format**: Markdown with clear headings and structure
- **Up-to-date**: Documentation updated alongside code changes

## 🤝 Contributing

When updating documentation:
1. Follow patterns in [`../CLAUDE.md`](../CLAUDE.md)
2. Keep docs current and relevant
3. Update cross-references when moving or renaming files
4. Include code examples where helpful
5. Test all commands and procedures before documenting

---

📍 **Quick Links**: [`../README.md`](../README.md) | [`ARCHITECTURE.md`](./ARCHITECTURE.md) | [`cs108/README.md`](./cs108/README.md) | [`../CLAUDE.md`](../CLAUDE.md)