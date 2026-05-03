/// OLS market model: fits r_i = alpha + beta * r_m + eps
/// Returns (alpha, beta, sigma_eps).
pub fn ols_market_model(r_i: &[f64], r_m: &[f64]) -> (f64, f64, f64) {
    assert_eq!(r_i.len(), r_m.len(), "r_i and r_m must have equal length");
    let n = r_i.len() as f64;
    assert!(n >= 3.0, "need at least 3 observations for OLS");

    let mean_ri = r_i.iter().sum::<f64>() / n;
    let mean_rm = r_m.iter().sum::<f64>() / n;

    let cov: f64 = r_i
        .iter()
        .zip(r_m.iter())
        .map(|(ri, rm)| (ri - mean_ri) * (rm - mean_rm))
        .sum();
    let var_rm: f64 = r_m.iter().map(|rm| (rm - mean_rm).powi(2)).sum();

    let beta = cov / var_rm;
    let alpha = mean_ri - beta * mean_rm;

    // sigma_eps = sqrt(SSR / (n-2))
    let ssr: f64 = r_i
        .iter()
        .zip(r_m.iter())
        .map(|(ri, rm)| {
            let fitted = alpha + beta * rm;
            (ri - fitted).powi(2)
        })
        .sum();
    let sigma_eps = (ssr / (n - 2.0)).sqrt();

    (alpha, beta, sigma_eps)
}

/// Market model abnormal return: AR_i = actual_i - (alpha + beta * market_i)
pub fn abnormal_return(actual: &[f64], market: &[f64], alpha: f64, beta: f64) -> Vec<f64> {
    assert_eq!(
        actual.len(),
        market.len(),
        "actual and market slices must have equal length"
    );
    actual
        .iter()
        .zip(market.iter())
        .map(|(a, m)| a - (alpha + beta * m))
        .collect()
}

/// Cumulative Abnormal Return (sum of AR over window).
pub fn cumulative_abnormal_return(ar: &[f64]) -> f64 {
    ar.iter().sum()
}

/// One-sample t-test against zero.
/// t = mean(samples) / (std_dev(samples) / sqrt(n))
/// Returns 0.0 if n < 2.
pub fn t_test_one_sample(samples: &[f64]) -> f64 {
    let n = samples.len();
    if n < 2 {
        return 0.0;
    }
    let nf = n as f64;
    let mean = samples.iter().sum::<f64>() / nf;
    let variance = samples
        .iter()
        .map(|x| (x - mean).powi(2))
        .sum::<f64>()
        / (nf - 1.0);
    let std_dev = variance.sqrt();
    if std_dev == 0.0 {
        return 0.0;
    }
    mean / (std_dev / nf.sqrt())
}

/// Boehmer-Musumeci-Poulsen test: t_BMP = mean(SCAR) / (std_dev(SCAR) / sqrt(N))
/// Delegates to t_test_one_sample.
pub fn bmp_test(standardized_cars: &[f64]) -> f64 {
    t_test_one_sample(standardized_cars)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_ols_market_model() {
        // Construct data where true alpha=0.01, beta=1.5
        // r_i = 0.01 + 1.5 * r_m (no noise)
        let r_m: Vec<f64> = vec![0.01, -0.02, 0.03, 0.005, -0.01, 0.02, 0.015, -0.005];
        let r_i: Vec<f64> = r_m.iter().map(|&m| 0.01 + 1.5 * m).collect();

        let (alpha, beta, sigma_eps) = ols_market_model(&r_i, &r_m);
        assert!((alpha - 0.01).abs() < 1e-6, "alpha = {}", alpha);
        assert!((beta - 1.5).abs() < 1e-6, "beta = {}", beta);
        // Perfect fit → sigma_eps ≈ 0
        assert!(sigma_eps < 1e-10, "sigma_eps = {}", sigma_eps);
    }

    #[test]
    fn test_abnormal_return() {
        let actual = [0.05, 0.03];
        let market = [0.02, 0.01];
        let ar = abnormal_return(&actual, &market, 0.0, 1.0);
        assert_eq!(ar.len(), 2);
        assert!((ar[0] - 0.03).abs() < 1e-10, "ar[0] = {}", ar[0]);
        assert!((ar[1] - 0.02).abs() < 1e-10, "ar[1] = {}", ar[1]);
    }

    #[test]
    fn test_cumulative_abnormal_return() {
        let ar = [0.01, 0.02, -0.005];
        let car = cumulative_abnormal_return(&ar);
        assert!((car - 0.025).abs() < 1e-10, "car = {}", car);
    }

    #[test]
    fn test_t_test_one_sample() {
        // mean=2, sd=1, n=3 → t = 2 / (1/sqrt(3)) = 2*sqrt(3) ≈ 3.4641016...
        let samples = [1.0, 2.0, 3.0];
        let t = t_test_one_sample(&samples);
        let expected = 2.0_f64 * 3.0_f64.sqrt();
        assert!((t - expected).abs() < 1e-6, "t = {}, expected = {}", t, expected);
    }

    #[test]
    fn test_t_test_one_sample_short() {
        assert_eq!(t_test_one_sample(&[]), 0.0);
        assert_eq!(t_test_one_sample(&[1.0]), 0.0);
    }

    #[test]
    fn test_bmp_test_delegates() {
        let samples = [1.0, 2.0, 3.0];
        assert_eq!(bmp_test(&samples), t_test_one_sample(&samples));
    }

    // Brown-Warner (1985): AR variance must include the (R_mt - R̄_m)² / Σ(R_ms - R̄_m)² term.
    // σ²(AR_it) = σ²_ε · [1 + 1/L1 + (R_mt - R̄_m)² / Σ_{s∈L1}(R_ms - R̄_m)²]
    #[test]
    fn test_bw_ar_variance() {
        // L1 market returns
        let r_m_l1: Vec<f64> = vec![0.01, -0.02, 0.03, 0.005, -0.01];
        let r_i_l1: Vec<f64> = r_m_l1.iter().map(|&m| 0.005 + 1.2 * m).collect();
        let (_alpha, _beta, sigma_eps) = ols_market_model(&r_i_l1, &r_m_l1);

        let l1 = r_m_l1.len() as f64;
        let mean_rm = r_m_l1.iter().sum::<f64>() / l1;
        let sum_sq_rm: f64 = r_m_l1.iter().map(|rm| (rm - mean_rm).powi(2)).sum();

        // Compute variance for a single event-window market return r_mt = 0.015
        let r_mt = 0.015_f64;
        let var_ar = sigma_eps.powi(2)
            * (1.0 + 1.0 / l1 + (r_mt - mean_rm).powi(2) / sum_sq_rm);

        // With perfect fit data sigma_eps ≈ 0, so var_ar ≈ 0; just verify formula runs and is ≥ 0
        assert!(var_ar >= 0.0, "BW variance must be non-negative, got {}", var_ar);

        // Non-trivial check: with noisy data sigma_eps > 0 → var_ar > 0
        let r_m2: Vec<f64> = vec![0.01, -0.02, 0.03, 0.005, -0.01, 0.02];
        let r_i2: Vec<f64> = vec![0.015, -0.025, 0.04, 0.003, -0.008, 0.018]; // not perfect fit
        let (_a2, _b2, sig2) = ols_market_model(&r_i2, &r_m2);
        let l2 = r_m2.len() as f64;
        let mean_rm2 = r_m2.iter().sum::<f64>() / l2;
        let sum_sq2: f64 = r_m2.iter().map(|rm| (rm - mean_rm2).powi(2)).sum();
        let var_ar2 = sig2.powi(2) * (1.0 + 1.0 / l2 + (r_mt - mean_rm2).powi(2) / sum_sq2);
        assert!(var_ar2 > 0.0, "expected positive BW variance for noisy data, got {}", var_ar2);
    }
}
