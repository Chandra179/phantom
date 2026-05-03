/// Euclidean distance between two slices.
#[no_mangle]
pub extern "C" fn euclidean_distance(
    a_ptr: *const f64, b_ptr: *const f64, length: u32, out: *mut f64,
) -> i32 { unimplemented!() }

/// Dynamic Time Warping distance.
#[no_mangle]
pub extern "C" fn dtw_distance(
    a_ptr: *const f64, a_len: u32, b_ptr: *const f64, b_len: u32, out: *mut f64,
) -> i32 { unimplemented!() }