use tonic::{transport::Server, Request, Response, Status};

pub mod compute {
    tonic::include_proto!("compute");
}

use compute::compute_service_server::{ComputeService, ComputeServiceServer};
use compute::*;

#[derive(Default)]
struct ComputeServiceImpl;

#[tonic::async_trait]
impl ComputeService for ComputeServiceImpl {
    async fn percent_changes(
        &self,
        _req: Request<PercentChangesRequest>,
    ) -> Result<Response<PercentChangesResponse>, Status> {
        Err(Status::unimplemented("percent_changes"))
    }

    async fn build_window(
        &self,
        _req: Request<BuildWindowRequest>,
    ) -> Result<Response<BuildWindowResponse>, Status> {
        Err(Status::unimplemented("build_window"))
    }

    async fn abnormal_return(
        &self,
        _req: Request<AbnormalReturnRequest>,
    ) -> Result<Response<AbnormalReturnResponse>, Status> {
        Err(Status::unimplemented("abnormal_return"))
    }

    async fn cumulative_abnormal_return(
        &self,
        _req: Request<CarRequest>,
    ) -> Result<Response<CarResponse>, Status> {
        Err(Status::unimplemented("cumulative_abnormal_return"))
    }

    async fn t_test_one_sample(
        &self,
        _req: Request<TTestRequest>,
    ) -> Result<Response<TTestResponse>, Status> {
        Err(Status::unimplemented("t_test_one_sample"))
    }
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let addr = "[::1]:50051".parse()?;
    println!("ComputeService listening on {addr}");
    Server::builder()
        .add_service(ComputeServiceServer::new(ComputeServiceImpl::default()))
        .serve(addr)
        .await?;
    Ok(())
}
