# Build stage
FROM public.ecr.aws/docker/library/python:3.13-slim AS builder

WORKDIR /app

# Install dependencies to user site-packages
COPY requirements.txt .
RUN pip install --no-cache-dir --user -r requirements.txt

# Final stage
FROM public.ecr.aws/docker/library/python:3.13-slim

WORKDIR /app

# Copy installed packages from builder
COPY --from=builder /root/.local /root/.local

# Set environment
ENV PATH=/root/.local/bin:$PATH
ENV PYTHONUNBUFFERED=1
ENV PORT=8080

# Copy application code
COPY . .

# Create non-root user and copy packages to their home
RUN useradd -m -u 1000 appuser && \
    cp -r /root/.local /home/appuser/.local && \
    chown -R appuser:appuser /home/appuser/.local /app

USER appuser
ENV PATH=/home/appuser/.local/bin:$PATH

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:8080/health')"

# Run the application
CMD ["python", "main.py"]
