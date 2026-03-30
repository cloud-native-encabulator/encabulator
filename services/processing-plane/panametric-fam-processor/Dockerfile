# Build stage
FROM public.ecr.aws/docker/library/node:22-alpine AS builder

WORKDIR /app

# Copy package files
COPY package*.json ./
COPY tsconfig.json ./

# Install dependencies
RUN npm ci

# Copy source code
COPY src ./src

# Build TypeScript
RUN npm run build

# Production stage
FROM public.ecr.aws/docker/library/node:22-alpine

WORKDIR /app

# Copy package files
COPY package*.json ./

# Install production dependencies only
RUN npm ci --only=production

# Copy built application
COPY --from=builder /app/dist ./dist

ENV NODE_ENV=production

# Expose port
EXPOSE 8080

USER node

# Run the application
CMD ["node", "dist/index.js"]
