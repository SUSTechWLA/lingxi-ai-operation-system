mod sandbox;

use std::net::SocketAddr;
use tonic::transport::Server;
use tracing_subscriber::EnvFilter;

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(EnvFilter::try_from_default_env().unwrap_or_else(|_| "info".into()))
        .init();

    let addr: SocketAddr = std::env::var("SANDBOX_ADDRESS")
        .unwrap_or_else(|_| "0.0.0.0:50051".into())
        .parse()?;

    let sandbox_svc = sandbox::SandboxServiceImpl::default();

    tracing::info!("Lingxi Sandbox starting on {}", addr);

    Server::builder()
        .add_service(sandbox::SandboxServiceServer::new(sandbox_svc))
        .serve(addr)
        .await?;

    Ok(())
}
