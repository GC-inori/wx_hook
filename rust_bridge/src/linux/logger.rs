use std::fs::{self, File, OpenOptions};
use std::io::Write;
use std::path::PathBuf;
use std::sync::Mutex;
use std::time::SystemTime;

static LOG_FILE: Mutex<Option<File>> = Mutex::new(None);

fn get_log_dir() -> PathBuf {
    if let Some(dir) = std::env::var_os("WX_BRIDGE_LOG_DIR") {
        return PathBuf::from(dir);
    }
    PathBuf::from("logs")
}

pub fn log_msg(level: &str, msg: &str) {
    let now = SystemTime::now()
        .duration_since(SystemTime::UNIX_EPOCH)
        .unwrap_or_default();
    let millis = now.subsec_millis();

    let timestamp = unsafe {
        let mut now_t: libc::time_t = 0;
        libc::time(&mut now_t);
        let mut tm: libc::tm = std::mem::zeroed();
        libc::localtime_r(&now_t, &mut tm);
        format!(
            "{:04}-{:02}-{:02} {:02}:{:02}:{:02}.{:03}",
            tm.tm_year + 1900,
            tm.tm_mon + 1,
            tm.tm_mday,
            tm.tm_hour,
            tm.tm_min,
            tm.tm_sec,
            millis
        )
    };

    let log_line = format!("{} [{}] {}\n", timestamp, level, msg);

    // 同时输出至 stderr
    eprint!("[rust_bridge] {}", log_line);

    // 写入文件 logs/bridge.log
    let mut file_guard = LOG_FILE.lock().unwrap_or_else(|p| p.into_inner());
    if file_guard.is_none() {
        let dir = get_log_dir();
        let _ = fs::create_dir_all(&dir);
        let log_path = dir.join("bridge.log");
        if let Ok(file) = OpenOptions::new().create(true).append(true).open(log_path) {
            *file_guard = Some(file);
        }
    }

    if let Some(file) = file_guard.as_mut() {
        let _ = file.write_all(log_line.as_bytes());
        let _ = file.flush();
    }
}

macro_rules! bridge_info {
    ($($arg:tt)*) => {
        $crate::linux::logger::log_msg("INFO", &format!($($arg)*))
    };
}
pub(crate) use bridge_info;

macro_rules! bridge_warn {
    ($($arg:tt)*) => {
        $crate::linux::logger::log_msg("WARN", &format!($($arg)*))
    };
}
pub(crate) use bridge_warn;

macro_rules! bridge_error {
    ($($arg:tt)*) => {
        $crate::linux::logger::log_msg("ERROR", &format!($($arg)*))
    };
}
pub(crate) use bridge_error;
