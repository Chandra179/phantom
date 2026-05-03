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
        req: Request<PercentChangesRequest>,
    ) -> Result<Response<PercentChangesResponse>, Status> {
        let prices = req.into_inner().prices;
        let result = std::panic::catch_unwind(|| graphic_processor::percent_changes(&prices));
        match result {
            Ok(Ok(changes)) => Ok(Response::new(PercentChangesResponse { changes })),
            Ok(Err(e)) => Err(Status::invalid_argument(e.to_string())),
            Err(_) => Err(Status::invalid_argument("percent_changes panicked")),
        }
    }

    async fn build_window(
        &self,
        req: Request<BuildWindowRequest>,
    ) -> Result<Response<BuildWindowResponse>, Status> {
        let inner = req.into_inner();
        let all_prices = inner.all_prices;
        let t0_index = inner.t0_index as usize;
        let est_days = inner.est_days;
        let event_hours = inner.event_hours;

        let result = std::panic::catch_unwind(move || {
            graphic_processor::build_window(&all_prices, t0_index, est_days, event_hours)
        });
        match result {
            Ok(window) => Ok(Response::new(BuildWindowResponse { window })),
            Err(_) => Err(Status::invalid_argument("build_window panicked")),
        }
    }

    async fn ols_market_model(
        &self,
        req: Request<OlsMarketModelRequest>,
    ) -> Result<Response<OlsMarketModelResponse>, Status> {
        let inner = req.into_inner();
        let r_i = inner.r_i;
        let r_m = inner.r_m;

        let result =
            std::panic::catch_unwind(move || backtesting::ols_market_model(&r_i, &r_m));
        match result {
            Ok((alpha, beta, sigma_eps)) => Ok(Response::new(OlsMarketModelResponse {
                alpha,
                beta,
                sigma_eps,
            })),
            Err(_) => Err(Status::invalid_argument("ols_market_model panicked")),
        }
    }

    async fn abnormal_return(
        &self,
        req: Request<AbnormalReturnRequest>,
    ) -> Result<Response<AbnormalReturnResponse>, Status> {
        let inner = req.into_inner();
        let actual = inner.actual_returns;
        let market = inner.market_returns;
        let alpha = inner.alpha;
        let beta = inner.beta;

        let result =
            std::panic::catch_unwind(move || backtesting::abnormal_return(&actual, &market, alpha, beta));
        match result {
            Ok(ar) => Ok(Response::new(AbnormalReturnResponse { ar })),
            Err(_) => Err(Status::invalid_argument("abnormal_return panicked")),
        }
    }

    async fn cumulative_abnormal_return(
        &self,
        req: Request<CarRequest>,
    ) -> Result<Response<CarResponse>, Status> {
        let ar = req.into_inner().ar;
        let result = std::panic::catch_unwind(move || backtesting::cumulative_abnormal_return(&ar));
        match result {
            Ok(car) => Ok(Response::new(CarResponse { car })),
            Err(_) => Err(Status::invalid_argument("cumulative_abnormal_return panicked")),
        }
    }

    async fn t_test_one_sample(
        &self,
        req: Request<TTestRequest>,
    ) -> Result<Response<TTestResponse>, Status> {
        let samples = req.into_inner().samples;
        let result = std::panic::catch_unwind(move || backtesting::t_test_one_sample(&samples));
        match result {
            Ok(t_statistic) => Ok(Response::new(TTestResponse { t_statistic })),
            Err(_) => Err(Status::invalid_argument("t_test_one_sample panicked")),
        }
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
