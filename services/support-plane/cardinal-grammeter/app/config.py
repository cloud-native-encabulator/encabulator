from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    app_name: str = "Cardinal Grammeter"
    secret_key: str = "your-secret-key-change-in-production"
    algorithm: str = "HS256"
    access_token_expire_minutes: int = 30
    
    class Config:
        env_file = ".env"


settings = Settings()
