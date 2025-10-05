import logging
from datetime import datetime, timedelta
from typing import List, Optional
import random

from .models import ResourceMetrics, CleanupResult
from .config import settings

logger = logging.getLogger(__name__)


class ResourceCollector:
    """Collects and manages resource cleanup operations"""
    
    def __init__(self):
        self.cleanup_history: List[CleanupResult] = []
    
    def collect_metrics(self, namespace: Optional[str] = None) -> List[ResourceMetrics]:
        """Collect resource metrics from Kubernetes"""
        # In a real implementation, this would use the Kubernetes API
        # For now, we'll generate mock data
        
        metrics = []
        resource_types = ['Pod', 'Deployment', 'Service', 'ConfigMap']
        namespaces = [namespace] if namespace else ['encabulator-system', 'encabulator-data', 'default']
        
        for ns in namespaces:
            for resource_type in resource_types:
                for i in range(random.randint(1, 5)):
                    metrics.append(ResourceMetrics(
                        resource_type=resource_type,
                        namespace=ns,
                        name=f"{resource_type.lower()}-{i}",
                        cpu_usage=random.uniform(0.05, 0.95),
                        memory_usage=random.uniform(0.1, 0.9),
                        age_hours=random.uniform(0.5, 72),
                        labels={'app': 'encabulator', 'managed-by': 'side-fumbling-collector'}
                    ))
        
        return metrics
    
    def cleanup_resources(
        self,
        resource_type: str,
        namespace: str,
        name: Optional[str] = None,
        dry_run: bool = True,
        age_threshold_hours: Optional[float] = None
    ) -> List[CleanupResult]:
        """Cleanup resources based on criteria"""
        
        if age_threshold_hours is None:
            age_threshold_hours = settings.resource_age_threshold_hours
        
        results = []
        
        # Collect current metrics
        metrics = self.collect_metrics(namespace)
        
        # Filter by resource type and name
        candidates = [
            m for m in metrics
            if m.resource_type == resource_type and
            (name is None or m.name == name) and
            m.age_hours > age_threshold_hours
        ]
        
        for candidate in candidates:
            if dry_run:
                result = CleanupResult(
                    resource_type=candidate.resource_type,
                    namespace=candidate.namespace,
                    name=candidate.name,
                    action='dry_run',
                    success=True,
                    reason=f'Would delete resource (age: {candidate.age_hours:.1f}h)',
                    timestamp=datetime.utcnow()
                )
            else:
                # In a real implementation, this would delete the resource via Kubernetes API
                logger.info(f"Cleaning up {candidate.resource_type}/{candidate.name} in {candidate.namespace}")
                
                result = CleanupResult(
                    resource_type=candidate.resource_type,
                    namespace=candidate.namespace,
                    name=candidate.name,
                    action='delete',
                    success=True,
                    reason=f'Deleted resource (age: {candidate.age_hours:.1f}h)',
                    timestamp=datetime.utcnow()
                )
            
            results.append(result)
            self.cleanup_history.append(result)
        
        # Keep only last 1000 cleanup results
        if len(self.cleanup_history) > 1000:
            self.cleanup_history = self.cleanup_history[-1000:]
        
        return results
    
    def get_cleanup_history(self, limit: int = 100) -> List[CleanupResult]:
        """Get cleanup history"""
        return self.cleanup_history[-limit:]
    
    def cleanup_old_resources(self) -> List[CleanupResult]:
        """Automated cleanup of old resources"""
        logger.info("Running automated cleanup")
        
        all_results = []
        resource_types = ['Pod', 'ConfigMap']
        
        for resource_type in resource_types:
            results = self.cleanup_resources(
                resource_type=resource_type,
                namespace=settings.namespace,
                dry_run=not settings.cleanup_enabled,
                age_threshold_hours=settings.resource_age_threshold_hours
            )
            all_results.extend(results)
        
        logger.info(f"Cleanup completed: {len(all_results)} resources processed")
        return all_results
