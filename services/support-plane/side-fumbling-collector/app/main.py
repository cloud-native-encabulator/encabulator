import logging
from datetime import datetime
from typing import List, Optional
from contextlib import asynccontextmanager

from fastapi import FastAPI, HTTPException, Query
from fastapi.responses import PlainTextResponse
from prometheus_client import Counter, Histogram, generate_latest, CONTENT_TYPE_LATEST
import uvicorn

from .config import settings
from .models import (
    HealthCheck,
    CleanupRequest,
    CleanupResult,
    AnalysisReport,
    OptimizationRecommendation,
    ResourceMetrics
)
from .collector import ResourceCollector
from .analyzer import ResourceAnalyzer
from .scheduler import TaskScheduler

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)

# Prometheus metrics
cleanup_counter = Counter('side_fumbling_cleanups_total', 'Total number of cleanup operations')
analysis_counter = Counter('side_fumbling_analyses_total', 'Total number of analysis operations')
cleanup_duration = Histogram('side_fumbling_cleanup_duration_seconds', 'Cleanup operation duration')

# Initialize components
collector = ResourceCollector()
analyzer = ResourceAnalyzer()
scheduler = TaskScheduler(collector, analyzer)


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Lifespan context manager for startup and shutdown"""
    # Startup
    logger.info(f"Starting {settings.app_name}")
    scheduler.start()
    yield
    # Shutdown
    logger.info(f"Shutting down {settings.app_name}")
    scheduler.stop()


# Create FastAPI app
app = FastAPI(
    title=settings.app_name,
    description="Resource optimization and cleanup service for Cloud Native Encabulator",
    version="1.0.0",
    lifespan=lifespan
)


@app.get("/health", response_model=HealthCheck)
async def health():
    """Health check endpoint"""
    return HealthCheck(status="healthy", timestamp=datetime.utcnow())


@app.get("/ready", response_model=HealthCheck)
async def ready():
    """Readiness check endpoint"""
    return HealthCheck(status="ready", timestamp=datetime.utcnow())


@app.get("/metrics", response_class=PlainTextResponse)
async def metrics():
    """Prometheus metrics endpoint"""
    return generate_latest()


@app.get("/api/v1/resources/metrics", response_model=List[ResourceMetrics])
async def get_resource_metrics(
    namespace: Optional[str] = Query(None, description="Filter by namespace")
):
    """Get current resource metrics"""
    try:
        metrics = collector.collect_metrics(namespace)
        return metrics
    except Exception as e:
        logger.error(f"Failed to collect metrics: {e}")
        raise HTTPException(status_code=500, detail=str(e))


@app.post("/api/v1/cleanup", response_model=List[CleanupResult])
async def cleanup_resources(request: CleanupRequest):
    """Manually trigger resource cleanup"""
    try:
        cleanup_counter.inc()
        
        with cleanup_duration.time():
            results = collector.cleanup_resources(
                resource_type=request.resource_type,
                namespace=request.namespace,
                name=request.name,
                dry_run=request.dry_run
            )
        
        return results
    except Exception as e:
        logger.error(f"Cleanup failed: {e}")
        raise HTTPException(status_code=500, detail=str(e))


@app.get("/api/v1/cleanup/history", response_model=List[CleanupResult])
async def get_cleanup_history(
    limit: int = Query(100, ge=1, le=1000, description="Number of results to return")
):
    """Get cleanup history"""
    return collector.get_cleanup_history(limit)


@app.get("/api/v1/analysis/report", response_model=AnalysisReport)
async def get_analysis_report():
    """Get resource analysis report"""
    try:
        analysis_counter.inc()
        
        # Collect fresh metrics
        metrics = collector.collect_metrics()
        for metric in metrics:
            analyzer.add_metrics(metric)
        
        report = analyzer.analyze()
        return report
    except Exception as e:
        logger.error(f"Analysis failed: {e}")
        raise HTTPException(status_code=500, detail=str(e))


@app.get("/api/v1/analysis/recommendations", response_model=List[OptimizationRecommendation])
async def get_recommendations():
    """Get optimization recommendations"""
    try:
        recommendations = analyzer.get_recommendations()
        return recommendations
    except Exception as e:
        logger.error(f"Failed to get recommendations: {e}")
        raise HTTPException(status_code=500, detail=str(e))


@app.post("/api/v1/analysis/run")
async def run_analysis():
    """Manually trigger resource analysis"""
    try:
        analysis_counter.inc()
        
        # Collect metrics
        metrics = collector.collect_metrics()
        for metric in metrics:
            analyzer.add_metrics(metric)
        
        # Generate report
        report = analyzer.analyze()
        
        return {
            "status": "completed",
            "timestamp": datetime.utcnow(),
            "resources_analyzed": report.total_resources
        }
    except Exception as e:
        logger.error(f"Analysis failed: {e}")
        raise HTTPException(status_code=500, detail=str(e))


if __name__ == "__main__":
    uvicorn.run(
        "app.main:app",
        host="0.0.0.0",
        port=settings.port,
        log_level="info"
    )
