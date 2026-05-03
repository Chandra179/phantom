/// Error type for price computation failures.
#[derive(Debug, PartialEq)]
pub enum PriceError {
    ZeroPrice { index: usize },
    NanPrice { index: usize },
}

impl std::fmt::Display for PriceError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            PriceError::ZeroPrice { index } => write!(f, "zero price at index {}", index),
            PriceError::NanPrice { index } => write!(f, "NaN price at index {}", index),
        }
    }
}

/// Computes log-returns: ln(prices[i] / prices[i-1]) for i in 1..n.
/// Returns a vec of len n-1. Returns empty vec for empty or single-element input.
/// Returns Err if any price is zero or NaN.
pub fn percent_changes(prices: &[f64]) -> Result<Vec<f64>, PriceError> {
    if prices.len() < 2 {
        return Ok(Vec::new());
    }
    let mut out = Vec::with_capacity(prices.len() - 1);
    for (i, w) in prices.windows(2).enumerate() {
        if w[0].is_nan() {
            return Err(PriceError::NanPrice { index: i });
        }
        if w[0] == 0.0 {
            return Err(PriceError::ZeroPrice { index: i });
        }
        out.push((w[1] / w[0]).ln());
    }
    Ok(out)
}

/// Builds a price window around T0.
/// Returns all_prices[t0_index - est_days .. t0_index + event_hours].
/// Panics if bounds are out of range.
pub fn build_window(
    all_prices: &[f64],
    t0_index: usize,
    est_days: u32,
    event_hours: u32,
) -> Vec<f64> {
    let start = t0_index - est_days as usize;
    let end = t0_index + event_hours as usize;
    all_prices[start..end].to_vec()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_percent_changes_basic() {
        let prices = [1.0, 2.0, 4.0];
        let changes = percent_changes(&prices).unwrap();
        assert_eq!(changes.len(), 2);
        let expected0 = (2.0_f64 / 1.0_f64).ln();
        let expected1 = (4.0_f64 / 2.0_f64).ln();
        assert!((changes[0] - expected0).abs() < 1e-10, "changes[0] = {}", changes[0]);
        assert!((changes[1] - expected1).abs() < 1e-10, "changes[1] = {}", changes[1]);
    }

    #[test]
    fn test_percent_changes_single() {
        let changes = percent_changes(&[42.0]).unwrap();
        assert!(changes.is_empty());
    }

    #[test]
    fn test_percent_changes_empty() {
        let changes = percent_changes(&[]).unwrap();
        assert!(changes.is_empty());
    }

    #[test]
    fn test_percent_changes_zero_price() {
        let prices = [1.0, 0.0, 4.0];
        let err = percent_changes(&prices).unwrap_err();
        assert_eq!(err, PriceError::ZeroPrice { index: 1 });
    }

    #[test]
    fn test_percent_changes_nan_price() {
        let prices = [f64::NAN, 2.0, 4.0];
        let err = percent_changes(&prices).unwrap_err();
        assert_eq!(err, PriceError::NanPrice { index: 0 });
    }

    #[test]
    fn test_build_window_basic() {
        let prices: Vec<f64> = (0..10).map(|x| x as f64).collect();
        // t0=5, est=3, event=2 → prices[2..7]
        let window = build_window(&prices, 5, 3, 2);
        let expected: Vec<f64> = vec![2.0, 3.0, 4.0, 5.0, 6.0];
        assert_eq!(window, expected);
    }
}
