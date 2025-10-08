import express from 'express';
import { register } from './utils/metrics';
import { logger } from './utils/logger';
import { DataPlaneClient } from './services/dataPlaneClient';
import { EncabulationProcessor } from './services/processor';
import { createEncabulationRouter } from './routes/encabulation';
import { createHealthRouter } from './routes/health';

const app = express();
const port = process.env.PORT || 8080;

// Middleware
app.use(express.json());

// Request logging middleware
app.use((req, res, next) => {
  const start = Date.now();
  res.on('finish', () => {
    const duration = Date.now() - start;
    logger.info('HTTP request', {
      method: req.method,
      path: req.path,
      status: res.statusCode,
      duration,
    });
  });
  next();
});

// Initialize services
const dataPlane = new DataPlaneClient();
const processor = new EncabulationProcessor(dataPlane);

// Routes
const healthRouter = createHealthRouter(dataPlane);
app.use('/', healthRouter);
app.use('/api/v1/encabulations', createEncabulationRouter(processor));

// Metrics endpoint
app.get('/metrics', async (req, res) => {
  res.set('Content-Type', register.contentType);
  res.end(await register.metrics());
});

// Error handling
app.use((err: Error, req: express.Request, res: express.Response, next: express.NextFunction) => {
  logger.error('Unhandled error', { error: err.message, stack: err.stack });
  res.status(500).json({
    error: 'Internal Server Error',
    code: 'UNHANDLED_ERROR',
    message: 'An unexpected error occurred',
  });
});

// Graceful shutdown
const server = app.listen(port, () => {
  logger.info(`Panametric Fam Processor started on port ${port}`);
});

process.on('SIGTERM', () => {
  logger.info('SIGTERM received, shutting down gracefully');
  server.close(() => {
    logger.info('Server closed');
    process.exit(0);
  });
});

process.on('SIGINT', () => {
  logger.info('SIGINT received, shutting down gracefully');
  server.close(() => {
    logger.info('Server closed');
    process.exit(0);
  });
});
