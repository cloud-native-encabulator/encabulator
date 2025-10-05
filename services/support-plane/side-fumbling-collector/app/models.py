from datetime import datetime
from typing import Any, Dict, List, Optional
from pydantic import BaseModel


class HealthCheck(BaseModel):
    """Health check response"""
    status: str
    timestamp: datetime


class ResourceMetrics(BaseModel):
    """Resource metrics data"""
    resource_type: str
    namespace: str
    name: str
    cpu_usage: Optional[float] = None
    memory_usage: Optional[float] = None
    age_hours: float
    labels: Dict[str, str] = {}


class CleanupResult(BaseModel):
    """Cleanup operation result"""
    resource_type: str
    namespace: str
    name: str
    action: str
    success: bool
    reason: Optional[str] = None
    timestamp: datetime


class AnalysisReport(BaseModel):
    """Resource analysis report"""
    timestamp: datetime
    total_resources: int
    underutilized_resources: int
    overutilized_resources: int
    cleanup_candidates: int
    recommendations: List[str]
    metrics_summary: Dict[str, float]


class CleanupRequest(BaseModel):
    """Manual cleanup request"""
    resource_type: str
    namespace: str
    name: Optional[str] = None
    dry_run: bool = True
    force: bool = False


class OptimizationRecommendation(BaseModel):
    """Resource optimization recommendation"""
    resource_type: str
    namespace: str
    name: str
    current_state: Dict[str, Any]
    recommended_state: Dict[str, Any]
    reason: str
    estimated_savings: Optional[float] = None
