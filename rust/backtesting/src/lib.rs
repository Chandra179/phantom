/// Market model abnormal return: AR_i = actual_i - (alpha + beta * market_i)
pub fn abnormal_return(
    _actual: &[f64],
    _market: &[f64],
    _alpha: f64,
    _beta: f64,
) -> Vec<f64> {
    unimplemented!()
}

/// Cumulative Abnormal Return (sum of AR over window).
pub fn cumulative_abnormal_return(_ar: &[f64]) -> f64 {
    unimplemented!()
}

/// One-sample t-test against zero.
pub fn t_test_one_sample(_samples: &[f64]) -> f64 {
    unimplemented!()
}
