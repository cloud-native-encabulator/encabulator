from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    """Application settings"""
    
    app_name: str = "Side Fumbling Collector"
    port: int = 8080
    environment: str = "development"
    
    # Kubernetes settings
    in_cluster: bool = False
    namespace: str = "encabulator-system"
    
    # Cleanup settings
    cleanup_enabled: bool = True
    cleanup_interval_minutes: int = 60
    resource_age_threshold_hours: int = 24
    
    # Analysis settings
    analysis_enabled: bool = True
    analysis_interval_minutes: int = 30
    
    class Config:
        env_file = ".env"
        case_sensitive = False


settings = Settings()
