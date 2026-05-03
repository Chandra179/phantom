fn main() {
    tonic_build::compile_protos("proto/compute.proto").unwrap();
}
