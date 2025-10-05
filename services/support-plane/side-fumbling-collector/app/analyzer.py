import logging
from datetime import datetime, timedelta
from typing import List, Dict
import numpy as np
import pandas as pd

from .models import ResourceMetrics, AnalysisReport, OptimizationRecommendation

logger = logging.getLogger(__name__)


class ResourceAnalyzer:
    """Analyzes resource usage and provides optimization recommendations"""
    
    def __init__(self):
        self.metrics_history: List[ResourceMetrics] = []
        self.cpu_threshold_low = 0.1  # 10% CPU usage
        self.cpu_threshold_high = 0.8  # 80% CPU usage
        self.memory_threshold_low = 0.2  # 20% memory usage
        self.memory_threshold_high = 0.85  # 85% memory usage
    
    def add_metrics(self, metrics: ResourceMetrics):
        """Add metrics to history"""
        self.metrics_history.append(metrics)
        
        # Keep only last 1000 entries
        if len(self.metrics_history) > 1000:
            self.metrics_history = self.metrics_history[-1000:]
    
    def analyze(self) -> AnalysisReport:
        """Perform resource analysis"""
        if not self.metrics_history:
            return AnalysisReport(
                timestamp=datetime.utcnow(),
                total_resources=0,
                underutilized_resources=0,
                overutilized_resources=0,
                cleanup_candidates=0,
                recommendations=[],
                metrics_summary={}
            )
        
        # Convert to DataFrame for analysis
        df = pd.DataFrame([
            {
                'resource_type': m.resource_type,
                'namespace': m.namespace,
                'name': m.name,
                'cpu_usage': m.cpu_usage or 0,
                'memory_usage': m.memory_usage or 0,
                'age_hours': m.age_hours
            }
            for m in self.metrics_history
        ])
        
        # Calculate statistics
        total_resources = len(df)
        underutilized = len(df[
            (df['cpu_usage'] < self.cpu_threshold_low) & 
            (df['memory_usage'] < self.memory_threshold_low)
        ])
        overutilized = len(df[
            (df['cpu_usage'] > self.cpu_threshold_high) | 
            (df['memory_usage'] > self.memory_threshold_high)
        ])
        
        # Find cleanup candidates (old and underutilized)
        cleanup_candidates = len(df[
            (df['age_hours'] > 24) & 
            (df['cpu_usage'] < self.cpu_threshold_low)
        ])
        
        # Generate recommendations
        recommendations = []
        if underutilized > 0:
            recommendations.append(
                f"Found {underutilized} underutilized resources - consider scaling down or removing"
            )
        if overutilized > 0:
            recommendations.append(
                f"Found {overutilized} overutilized resources - consider scaling up"
            )
        if cleanup_candidates > 0:
            recommendations.append(
                f"Found {cleanup_candidates} resources eligible for cleanup"
            )
        
        # Calculate metrics summary
        metrics_summary = {
            'avg_cpu_usage': float(df['cpu_usage'].mean()),
            'avg_memory_usage': float(df['memory_usage'].mean()),
            'max_cpu_usage': float(df['cpu_usage'].max()),
            'max_memory_usage': float(df['memory_usage'].max()),
            'avg_age_hours': float(df['age_hours'].mean())
        }
        
        return AnalysisReport(
            timestamp=datetime.utcnow(),
            total_resources=total_resources,
            underutilized_resources=underutilized,
            overutilized_resources=overutilized,
            cleanup_candidates=cleanup_candidates,
            recommendations=recommendations,
            metrics_summary=metrics_summary
        )
    
    def get_recommendations(self) -> List[OptimizationRecommendation]:
        """Get specific optimization recommendations"""
        recommendations = []
        
        for metrics in self.metrics_history[-100:]:  # Last 100 resources
            # Check for underutilization
            if (metrics.cpu_usage and metrics.cpu_usage < self.cpu_threshold_low and
                metrics.memory_usage and metrics.memory_usage < self.memory_threshold_low):
                
                recommendations.append(OptimizationRecommendation(
                    resource_type=metrics.resource_type,
                    namespace=metrics.namespace,
                    name=metrics.name,
                    current_state={
                        'cpu_usage': metrics.cpu_usage,
                        'memory_usage': metrics.memory_usage
                    },
                    recommended_state={
                        'action': 'scale_down',
                        'target_replicas': 1
                    },
                    reason='Resource is underutilized',
                    estimated_savings=0.5
                ))
            
            # Check for overutilization
            elif (metrics.cpu_usage and metrics.cpu_usage > self.cpu_threshold_high or
                  metrics.memory_usage and metrics.memory_usage > self.memory_threshold_high):
                
                recommendations.append(OptimizationRecommendation(
                    resource_type=metrics.resource_type,
                    namespace=metrics.namespace,
                    name=metrics.name,
                    current_state={
                        'cpu_usage': metrics.cpu_usage,
                        'memory_usage': metrics.memory_usage
                    },
                    recommended_state={
                        'action': 'scale_up',
                        'increase_resources': '50%'
                    },
                    reason='Resource is overutilized',
                    estimated_savings=-0.5
                ))
        
        return recommendations
