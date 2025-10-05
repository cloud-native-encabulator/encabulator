import pytest
from datetime import datetime

from app.analyzer import ResourceAnalyzer
from app.models import ResourceMetrics


def test_analyzer_initialization():
    """Test analyzer initialization"""
    analyzer = ResourceAnalyzer()
    assert analyzer is not None
    assert len(analyzer.metrics_history) == 0


def test_add_metrics():
    """Test adding metrics to analyzer"""
    analyzer = ResourceAnalyzer()
    
    metric = ResourceMetrics(
        resource_type="Pod",
        namespace="test",
        name="test-pod",
        cpu_usage=0.5,
        memory_usage=0.6,
        age_hours=12.0
    )
    
    analyzer.add_metrics(metric)
    assert len(analyzer.metrics_history) == 1


def test_analyze_empty():
    """Test analysis with no metrics"""
    analyzer = ResourceAnalyzer()
    report = analyzer.analyze()
    
    assert report.total_resources == 0
    assert report.underutilized_resources == 0
    assert report.overutilized_resources == 0


def test_analyze_with_metrics():
    """Test analysis with metrics"""
    analyzer = ResourceAnalyzer()
    
    # Add underutilized resource
    analyzer.add_metrics(ResourceMetrics(
        resource_type="Pod",
        namespace="test",
        name="underutilized-pod",
        cpu_usage=0.05,
        memory_usage=0.1,
        age_hours=12.0
    ))
    
    # Add overutilized resource
    analyzer.add_metrics(ResourceMetrics(
        resource_type="Pod",
        namespace="test",
        name="overutilized-pod",
        cpu_usage=0.9,
        memory_usage=0.9,
        age_hours=6.0
    ))
    
    # Add normal resource
    analyzer.add_metrics(ResourceMetrics(
        resource_type="Pod",
        namespace="test",
        name="normal-pod",
        cpu_usage=0.5,
        memory_usage=0.5,
        age_hours=3.0
    ))
    
    report = analyzer.analyze()
    
    assert report.total_resources == 3
    assert report.underutilized_resources >= 1
    assert report.overutilized_resources >= 1
    assert len(report.recommendations) > 0


def test_get_recommendations():
    """Test getting optimization recommendations"""
    analyzer = ResourceAnalyzer()
    
    # Add underutilized resource
    analyzer.add_metrics(ResourceMetrics(
        resource_type="Pod",
        namespace="test",
        name="underutilized-pod",
        cpu_usage=0.05,
        memory_usage=0.1,
        age_hours=12.0
    ))
    
    recommendations = analyzer.get_recommendations()
    
    assert len(recommendations) > 0
    assert recommendations[0].resource_type == "Pod"
    assert recommendations[0].name == "underutilized-pod"


def test_metrics_history_limit():
    """Test that metrics history is limited"""
    analyzer = ResourceAnalyzer()
    
    # Add more than 1000 metrics
    for i in range(1100):
        analyzer.add_metrics(ResourceMetrics(
            resource_type="Pod",
            namespace="test",
            name=f"pod-{i}",
            cpu_usage=0.5,
            memory_usage=0.5,
            age_hours=1.0
        ))
    
    # Should be limited to 1000
    assert len(analyzer.metrics_history) == 1000
