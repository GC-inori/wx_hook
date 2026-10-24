use std::path::PathBuf;

fn main() {
    let target_os = std::env::var("CARGO_CFG_TARGET_OS").unwrap_or_default();
    if target_os != "windows" {
        // MinHook and the SEH-based scheduler helper are Windows-only.
        return;
    }

    let manifest_dir = PathBuf::from(std::env::var("CARGO_MANIFEST_DIR").unwrap());
    let minhook_dir = manifest_dir.join("../native/vendor/minhook-1.3.4");

    cc::Build::new()
        .include(minhook_dir.join("include"))
        .include(minhook_dir.join("src"))
        .include(minhook_dir.join("src/hde"))
        .file(minhook_dir.join("src/buffer.c"))
        .file(minhook_dir.join("src/hook.c"))
        .file(minhook_dir.join("src/trampoline.c"))
        .file(minhook_dir.join("src/hde/hde64.c"))
        .compile("minhook");

    cc::Build::new()
        .cpp(true)
        .file(manifest_dir.join("src/native_helper.cpp"))
        .compile("native_helper");

    println!("cargo:rerun-if-changed=src/native_helper.cpp");
    println!("cargo:rerun-if-changed=../native/vendor/minhook-1.3.4");
}
