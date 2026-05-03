use std::any::Any;
use tonic::{transport::Server, Request, Response, Status};

pub mod compute {
    tonic::include_proto!("compute");
}

use compute::compute_service_server::{ComputeService, ComputeServiceServer};
use compute::*;

fn panic_msg(name: &str, payload: Box<dyn Any + Send>) -> Status {
    let detail = payload
        .downcast_ref::<String>()
        .map(|s| s.as_str())
        .or_else(|| payload.downcast_ref::<&str>().copied())
        .unwrap_or("<no message>");
    Status::invalid_argument(format!("{name} panicked: {detail}"))
}

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
            Err(p) => Err(panic_msg("percent_changes", p)),
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
            Err(p) => Err(panic_msg("build_window", p)),
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
            Err(p) => Err(panic_msg("ols_market_model", p)),
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
            Err(p) => Err(panic_msg("abnormal_return", p)),
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
            Err(p) => Err(panic_msg("cumulative_abnormal_return", p)),
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
            Err(p) => Err(panic_msg("t_test_one_sample", p)),
        }
    }

    async fn euclidean_distance(
        &self,
        req: Request<EuclideanDistanceRequest>,
    ) -> Result<Response<EuclideanDistanceResponse>, Status> {
        let inner = req.into_inner();
        let a = inner.a;
        let b = inner.b;

        let result = std::panic::catch_unwind(move || shape_matching::euclidean_distance(&a, &b));
        match result {
            Ok(distance) => Ok(Response::new(EuclideanDistanceResponse { distance })),
            Err(p) => Err(panic_msg("euclidean_distance", p)),
        }
    }

    async fn dtw_distance(
        &self,
        req: Request<DtwDistanceRequest>,
    ) -> Result<Response<DtwDistanceResponse>, Status> {
        let inner = req.into_inner();
        let a = inner.a;
        let b = inner.b;
        let band = inner.band as usize;

        let result =
            std::panic::catch_unwind(move || shape_matching::dtw_distance(&a, &b, band));
        match result {
            Ok(distance) => Ok(Response::new(DtwDistanceResponse { distance })),
            Err(p) => Err(panic_msg("dtw_distance", p)),
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
