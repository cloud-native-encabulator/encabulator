import pytest

from app.collector import ResourceCollector


def test_collector_initialization():
    """Test collector initialization"""
    collector = ResourceCollector()
    assert collector is not None
    assert len(collector.cleanup_history) == 0


def test_collect_metrics():
    """Test collecting metrics"""
    collector = ResourceCollector()
    metrics = collector.collect_metrics()
    
    assert len(metrics) > 0
    assert all(m.resource_type in ['Pod', 'Deployment', 'Service', 'ConfigMap'] for m in metrics)


def test_collect_metrics_with_namespace():
    """Test collecting metrics for specific namespace"""
    collector = ResourceCollector()
    metrics = collector.collect_metrics(namespace="encabulator-system")
    
    assert len(metrics) > 0
    assert all(m.namespace == "encabulator-system" for m in metrics)


def test_cleanup_dry_run():
    """Test cleanup in dry run mode"""
    collector = ResourceCollector()
    
    results = collector.cleanup_resources(
        resource_type="Pod",
        namespace="encabulator-system",
        dry_run=True,
        age_threshold_hours=0  # All resources are candidates
    )
    
    assert len(results) >= 0
    for result in results:
        assert result.action == "dry_run"
        assert result.success is True


def test_cleanup_with_name():
    """Test cleanup of specific resource"""
    collector = ResourceCollector()
    
    results = collector.cleanup_resources(
        resource_type="Pod",
        namespace="encabulator-system",
        name="specific-pod",
        dry_run=True
    )
    
    # Should only return results for the specific name if it exists
    assert all(r.name == "specific-pod" for r in results)


def test_cleanup_history():
    """Test cleanup history tracking"""
    collector = ResourceCollector()
    
    # Perform cleanup
    collector.cleanup_resources(
        resource_type="Pod",
        namespace="encabulator-system",
        dry_run=True,
        age_threshold_hours=0
    )
    
    history = collector.get_cleanup_history()
    assert len(history) > 0


def test_cleanup_history_limit():
    """Test that cleanup history is limited"""
    collector = ResourceCollector()
    
    # Add more than 1000 cleanup results
    for i in range(1100):
        collector.cleanup_resources(
            resource_type="Pod",
            namespace="test",
            dry_run=True,
            age_threshold_hours=0
        )
    
    # History should be limited
    assert len(collector.cleanup_history) <= 1000


def test_cleanup_old_resources():
    """Test automated cleanup of old resources"""
    collector = ResourceCollector()
    
    results = collector.cleanup_old_resources()
    
    assert isinstance(results, list)
    # Results may be empty if no old resources found
