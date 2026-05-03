/// Z-normalize a slice: subtract mean, divide by std (pop).
/// Returns normalized copy. Returns all zeros if std == 0.
pub fn z_normalize(data: &[f64]) -> Vec<f64> {
    let n = data.len();
    if n == 0 {
        return Vec::new();
    }
    let nf = n as f64;
    let mean = data.iter().sum::<f64>() / nf;
    let variance = data.iter().map(|x| (x - mean).powi(2)).sum::<f64>() / nf;
    let std = variance.sqrt();
    if std == 0.0 {
        return vec![0.0; n];
    }
    data.iter().map(|x| (x - mean) / std).collect()
}

/// Euclidean distance between two time series.
/// Both series z-normalized before comparison.
pub fn euclidean_distance(a: &[f64], b: &[f64]) -> f64 {
    assert_eq!(a.len(), b.len(), "slices must have equal length");
    if a.is_empty() {
        return 0.0;
    }
    let a_norm = z_normalize(a);
    let b_norm = z_normalize(b);
    let sum_sq: f64 = a_norm
        .iter()
        .zip(b_norm.iter())
        .map(|(x, y)| (x - y).powi(2))
        .sum();
    sum_sq.sqrt()
}

/// DTW distance with Sakoe-Chiba band constraint.
/// band = max allowable |i - j| shift. Both series z-normalized.
pub fn dtw_distance(a: &[f64], b: &[f64], band: usize) -> f64 {
    let a_norm = z_normalize(a);
    let b_norm = z_normalize(b);
    dtw_distance_normalized(&a_norm, &b_norm, band)
}

fn dtw_distance_normalized(a: &[f64], b: &[f64], band: usize) -> f64 {
    let n = a.len();
    let m = b.len();
    if n == 0 && m == 0 {
        return 0.0;
    }
    if n == 0 || m == 0 {
        return f64::INFINITY;
    }

    let inf = f64::INFINITY;
    let mut prev = vec![inf; m + 1];
    prev[0] = 0.0;

    for i in 1..=n {
        let mut curr = vec![inf; m + 1];
        let j_start = if i > band { i - band } else { 1 };
        let j_end = (i + band).min(m);
        for j in j_start..=j_end {
            let cost = (a[i - 1] - b[j - 1]).powi(2);
            let min_prev = prev[j].min(curr[j - 1]).min(prev[j - 1]);
            curr[j] = cost + min_prev;
        }
        prev = curr;
    }
    prev[m].sqrt()
}

/// LB_Keogh lower bound for DTW.
/// Guaranteed: lb_keogh(q, c, band) <= dtw_distance(q, c, band).
/// Both query and candidate z-normalized.
pub fn lb_keogh(query: &[f64], candidate: &[f64], band: usize) -> f64 {
    let q_norm = z_normalize(query);
    let c_norm = z_normalize(candidate);
    lb_keogh_normalized(&q_norm, &c_norm, band)
}

fn lb_keogh_normalized(query: &[f64], candidate: &[f64], band: usize) -> f64 {
    assert_eq!(
        query.len(),
        candidate.len(),
        "query and candidate must have equal length"
    );
    let n = query.len();
    if n == 0 {
        return 0.0;
    }

    let mut upper = vec![f64::NEG_INFINITY; n];
    let mut lower = vec![f64::INFINITY; n];
    for i in 0..n {
        let start = i.saturating_sub(band);
        let end = (i + band + 1).min(n);
        for &val in query[start..end].iter() {
            if val > upper[i] {
                upper[i] = val;
            }
            if val < lower[i] {
                lower[i] = val;
            }
        }
    }
    let mut sum_sq = 0.0;
    for i in 0..n {
        if candidate[i] > upper[i] {
            sum_sq += (candidate[i] - upper[i]).powi(2);
        } else if candidate[i] < lower[i] {
            sum_sq += (candidate[i] - lower[i]).powi(2);
        }
    }
    sum_sq.sqrt()
}

/// Find candidates matching query within threshold.
/// Uses LB_Keogh prune first, then full DTW on survivors.
/// Normalizes once per candidate (not twice) for efficiency.
/// Returns (idx, dtw_dist) for matches below threshold.
pub fn find_matches(
    query: &[f64],
    candidates: &[Vec<f64>],
    band: usize,
    threshold: f64,
) -> Vec<(usize, f64)> {
    let mut results = Vec::new();
    let q_norm = z_normalize(query);
    for (i, c) in candidates.iter().enumerate() {
        let c_norm = z_normalize(c);
        let lb = lb_keogh_normalized(&q_norm, &c_norm, band);
        if lb <= threshold {
            let d = dtw_distance_normalized(&q_norm, &c_norm, band);
            if d <= threshold {
                results.push((i, d));
            }
        }
    }
    results
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_euclidean_distance_znorm_golden() {
        let a = vec![1.0, 2.0, 3.0];
        let b = vec![3.0, 2.0, 1.0];
        let d = euclidean_distance(&a, &b);
        let expected = 3.4641016151377544;
        assert!((d - expected).abs() < 1e-10, "got {} expected {}", d, expected);
    }

    #[test]
    fn test_euclidean_distance_empty() {
        assert_eq!(euclidean_distance(&[], &[]), 0.0);
    }

    #[test]
    fn test_euclidean_distance_identical() {
        let a = vec![1.0, 2.0, 3.0, 4.0, 5.0];
        assert!(euclidean_distance(&a, &a) < 1e-12);
    }

    #[test]
    fn test_dtw_distance_sakoe_chiba_golden() {
        // Peak vs valley; band=1 enables warping to match peak-to-peak, valley-to-valley
        let a = vec![1.0, 10.0, 1.0];
        let b = vec![10.0, 1.0, 10.0];
        let d0 = dtw_distance(&a, &b, 0);
        let d1 = dtw_distance(&a, &b, 1);
        assert!(
            d1 < d0,
            "band=1 distance {} should be < band=0 distance {}",
            d1,
            d0
        );
        let expected = 2.23606797749979;
        assert!((d1 - expected).abs() < 1e-10, "got {} expected {}", d1, expected);
    }

    #[test]
    fn test_dtw_distance_identical() {
        let a = vec![1.0, 2.0, 3.0, 4.0, 5.0];
        assert!(dtw_distance(&a, &a, 2) < 1e-12);
    }

    #[test]
    fn test_dtw_distance_empty() {
        assert_eq!(dtw_distance(&[], &[], 1), 0.0);
        assert_eq!(dtw_distance(&[1.0], &[], 1), f64::INFINITY);
        assert_eq!(dtw_distance(&[], &[1.0], 1), f64::INFINITY);
    }

    #[test]
    fn test_dtw_band_constraint() {
        // Wider band should give smaller or equal distance
        let a = vec![1.0, 10.0, 1.0];
        let b = vec![10.0, 1.0, 10.0];
        let d0 = dtw_distance(&a, &b, 0);
        let d1 = dtw_distance(&a, &b, 1);
        let d2 = dtw_distance(&a, &b, 2);
        assert!(d0 >= d1, "band=0 dist {} > band=1 dist {}", d0, d1);
        assert!(d1 >= d2, "band=1 dist {} > band=2 dist {}", d1, d2);
    }

    #[test]
    fn test_lb_keogh_bounds_dtw() {
        // Property: LB_Keogh <= true DTW for any pair
        use rand::Rng;
        let mut rng = rand::thread_rng();
        for _ in 0..1000 {
            let n = rng.gen_range(10..=50);
            let q: Vec<f64> = (0..n).map(|_| rng.gen_range(-10.0..10.0)).collect();
            let c: Vec<f64> = (0..n).map(|_| rng.gen_range(-10.0..10.0)).collect();
            let band = rng.gen_range(1..=n / 4).max(1);
            let lb = lb_keogh(&q, &c, band);
            let dt = dtw_distance(&q, &c, band);
            assert!(
                lb <= dt + 1e-9,
                "LB_Keogh {} > DTW {} for n={} band={}",
                lb,
                dt,
                n,
                band
            );
        }
    }

    #[test]
    fn test_lb_keogh_self() {
        // LB_Keogh(q, q) should be 0
        let q = vec![1.0, 2.0, 3.0, 4.0, 5.0];
        assert!(lb_keogh(&q, &q, 1) < 1e-12);
    }

    #[test]
    fn test_prune_workflow() {
        let query = vec![1.0, 2.0, 3.0, 4.0, 5.0];
        let candidates: Vec<Vec<f64>> = vec![
            vec![1.0, 2.0, 3.0, 4.0, 5.0],
            vec![5.0, 4.0, 3.0, 2.0, 1.0],
            vec![3.0, 3.0, 3.0, 3.0, 3.0],
            vec![1.0, 3.0, 5.0, 3.0, 1.0],
            vec![5.0, 3.0, 1.0, 3.0, 5.0],
            vec![10.0, 8.0, 6.0, 4.0, 2.0],
            vec![2.0, 4.0, 6.0, 8.0, 10.0],
            vec![0.0, 2.0, 7.0, 2.0, 0.0],
            vec![1.0, 5.0, 2.0, 6.0, 1.0],
            vec![9.0, 7.0, 5.0, 3.0, 1.0],
        ];
        let band = 1;
        let threshold = 3.0;

        // Verify LB_Keogh individually
        let lbs: Vec<f64> = candidates
            .iter()
            .map(|c| lb_keogh(&query, c, band))
            .collect();
        let expected_lbs = vec![
            0.0, 3.162_277_660_168_379, 1.0, 2.007_432_524_319_517_7, 2.007_432_524_319_517_7,
            3.162_277_660_168_379, 0.0, 1.954_606_794_729_467_9, 1.914_898_669_325_193,
            3.162_277_660_168_379,
        ];
        for (i, (&got, &exp)) in lbs.iter().zip(expected_lbs.iter()).enumerate() {
            assert!(
                (got - exp).abs() < 1e-6,
                "candidate {} LB_Keogh: got {} expected {}",
                i,
                got,
                exp
            );
        }

        // LB_Keogh reject count: those with LB > threshold
        let rejected = lbs.iter().filter(|&&lb| lb > threshold).count();
        assert_eq!(rejected, 3, "expected 3 rejected by LB_Keogh, got {}", rejected);

        // find_matches should apply LB_Keogh prune then DTW
        let matches = find_matches(&query, &candidates, band, threshold);
        // candidates 0,2,3,4,6,7,8 should pass LB_Keogh threshold (3.0)
        // after DTW, all have DTW <= threshold? Let's verify:
        let dtw_all: Vec<f64> = candidates
            .iter()
            .map(|c| dtw_distance(&query, c, band))
            .collect();
        let true_matches: Vec<(usize, f64)> = dtw_all
            .iter()
            .enumerate()
            .filter(|(_, &d)| d <= threshold)
            .map(|(i, &d)| (i, d))
            .collect();
        assert_eq!(
            matches, true_matches,
            "find_matches returned {:?}, expected {:?}",
            matches, true_matches
        );
        // Indices that survived DTW: 0,2,3,4,6,7,8 (all with DTW <= 3.0)
        assert_eq!(matches.len(), 7);
    }

    #[test]
    fn test_z_normalize_basic() {
        let data = vec![1.0, 2.0, 3.0];
        let norm = z_normalize(&data);
        assert!((norm[0] - (-1.224744871391589)).abs() < 1e-10);
        assert!((norm[1] - 0.0).abs() < 1e-10);
        assert!((norm[2] - 1.224744871391589).abs() < 1e-10);
    }

    #[test]
    fn test_z_normalize_constant() {
        let data = vec![5.0, 5.0, 5.0];
        let norm = z_normalize(&data);
        assert_eq!(norm, vec![0.0, 0.0, 0.0]);
    }

    #[test]
    fn test_z_normalize_empty() {
        assert!(z_normalize(&[]).is_empty());
    }
}
