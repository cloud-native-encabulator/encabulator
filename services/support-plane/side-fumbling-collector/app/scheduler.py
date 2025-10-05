import logging
from apscheduler.schedulers.background import BackgroundScheduler
from apscheduler.triggers.interval import IntervalTrigger

from .config import settings
from .collector import ResourceCollector
from .analyzer import ResourceAnalyzer

logger = logging.getLogger(__name__)


class TaskScheduler:
    """Schedules periodic cleanup and analysis tasks"""
    
    def __init__(self, collector: ResourceCollector, analyzer: ResourceAnalyzer):
        self.collector = collector
        self.analyzer = analyzer
        self.scheduler = BackgroundScheduler()
    
    def start(self):
        """Start scheduled tasks"""
        logger.info("Starting task scheduler")
        
        # Schedule cleanup task
        if settings.cleanup_enabled:
            self.scheduler.add_job(
                func=self._run_cleanup,
                trigger=IntervalTrigger(minutes=settings.cleanup_interval_minutes),
                id='cleanup_task',
                name='Resource Cleanup',
                replace_existing=True
            )
            logger.info(f"Scheduled cleanup task every {settings.cleanup_interval_minutes} minutes")
        
        # Schedule analysis task
        if settings.analysis_enabled:
            self.scheduler.add_job(
                func=self._run_analysis,
                trigger=IntervalTrigger(minutes=settings.analysis_interval_minutes),
                id='analysis_task',
                name='Resource Analysis',
                replace_existing=True
            )
            logger.info(f"Scheduled analysis task every {settings.analysis_interval_minutes} minutes")
        
        self.scheduler.start()
    
    def stop(self):
        """Stop scheduled tasks"""
        logger.info("Stopping task scheduler")
        self.scheduler.shutdown()
    
    def _run_cleanup(self):
        """Run cleanup task"""
        try:
            logger.info("Running scheduled cleanup")
            results = self.collector.cleanup_old_resources()
            logger.info(f"Cleanup completed: {len(results)} resources processed")
        except Exception as e:
            logger.error(f"Cleanup task failed: {e}")
    
    def _run_analysis(self):
        """Run analysis task"""
        try:
            logger.info("Running scheduled analysis")
            
            # Collect current metrics
            metrics = self.collector.collect_metrics()
            
            # Add to analyzer
            for metric in metrics:
                self.analyzer.add_metrics(metric)
            
            # Generate report
            report = self.analyzer.analyze()
            logger.info(f"Analysis completed: {report.total_resources} resources analyzed")
            
        except Exception as e:
            logger.error(f"Analysis task failed: {e}")
