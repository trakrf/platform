# Railway Deployment Guide

## Prerequisites
- Railway account
- GitHub repository connected to Railway

## Configuration

### Environment Variables

#### Build Configuration (Required)
These handle native module compilation issues during deployment:

```bash
# Skip native module compilation (not needed for production)
RAILWAY_BUILD_INSTALL_COMMAND="pnpm install --frozen-lockfile --ignore-scripts"

# Alternative: Use the install script
# RAILWAY_BUILD_INSTALL_COMMAND="bash scripts/railway-install.sh"

# Standard Railway variables (usually auto-detected)
RAILWAY_BUILD_COMMAND="pnpm build"
RAILWAY_START_COMMAND="pnpm start"
```

#### Runtime Variables (Optional)
- `PORT` - Automatically set by Railway
- `NODE_ENV` - Set to 'production' by Railway

### Why Skip Native Modules?

The following native modules are development dependencies that require node-gyp and C++ compilation:
- `@stoprocent/noble` - Bluetooth LE library for local device testing
- `@serialport/bindings-cpp` - Serial port support
- `@stoprocent/bluetooth-hci-socket` - Low-level Bluetooth access
- `usb` - USB device access

These are only used for local development and testing with real hardware. The production web app uses Web Bluetooth API instead.

### Build Settings
- **Build Command**: `pnpm build` (via RAILWAY_BUILD_COMMAND)
- **Start Command**: `pnpm start` (via RAILWAY_START_COMMAND)
- **Node Version**: 18+ (use engines field in package.json)

### Domain Configuration
1. Add custom domain in Railway settings
2. **Important**: Set port to "Auto detect (8080)" instead of overriding to port 80
3. Configure HTTPS (automatic with Railway)

## Deployment Process
1. Push to main branch
2. Railway automatically builds and deploys
3. Monitor deployment logs for errors

## Bundle Optimization

The application uses code splitting to reduce initial load time:

### Lazy Loading Implementation
- **CS108 Managers**: Loaded on-demand when device operations are needed
- **Screen Components**: Loaded when navigating to each screen
- **Vendor Chunks**: Separated into smaller, cacheable chunks

### Bundle Structure
- `react-vendor`: React core libraries
- `ui-vendor`: UI components (Headless UI, Toast, etc.)
- `icons`: React Icons library
- `gauge`: Gauge component (used in Settings)
- `cs108-*`: RFID reader protocol modules

## Troubleshooting

### Native Module Build Errors
**Cause**: Railway trying to compile unnecessary native modules
**Solution**: 
- Ensure `RAILWAY_BUILD_INSTALL_COMMAND` is set with `--ignore-scripts`
- Alternative: Use `pnpm run install:production` script

### 502 Bad Gateway Error
**Cause**: No server running after build completes or incorrect port configuration
**Solution**: 
- Ensure `pnpm start` script exists and uses `serve`
- Set custom domain port to "Auto detect (8080)" instead of overriding to port 80

### Large Bundle Warnings
**Cause**: All code loaded in main bundle
**Solution**: Implement code splitting as configured in `vite.config.ts`

### 404 on Routes
**Cause**: Server not configured for SPA routing
**Solution**: The `serve -s` flag handles this automatically

### Module Not Found Errors
**Cause**: Incorrect import paths after refactoring
**Solution**: Verify all lazy import paths match file structure

## Performance Monitoring

### Check Bundle Sizes
```bash
pnpm build 2>&1 | grep -E "dist/assets" | grep -E "\.js"
```

### Analyze Chunks
Look for warnings about chunks larger than 500KB:
```bash
pnpm build 2>&1 | grep -E "chunks are larger than 500"
```

### Test Production Build Locally
```bash
# Build the application
pnpm build

# Serve locally on port 3000
PORT=3000 pnpm start
```

## Security Considerations

1. **Environment Variables**: Never commit sensitive data
2. **HTTPS**: Always enabled on Railway
3. **Content Security Policy**: Consider adding CSP headers
4. **API Keys**: Use Railway's environment variable management

## Monitoring & Logs

### Railway Dashboard
- View deployment logs in real-time
- Monitor resource usage
- Set up alerts for failures

### Application Logs
- Console logs are captured by Railway
- Use structured logging for better searchability
- Monitor error rates and performance metrics

## Rollback Strategy

If deployment fails:
1. Use Railway's deployment history
2. Rollback to previous successful deployment
3. Fix issues in development before redeploying

## Additional Resources

- [Railway React Guide](https://docs.railway.com/guides/react)
- [Vite Production Guide](https://vite.dev/guide/build.html)
- [Serve Documentation](https://www.npmjs.com/package/serve)