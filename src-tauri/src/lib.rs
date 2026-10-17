use std::sync::Mutex;

use serde::{Deserialize, Serialize};
use tauri::Manager;
use tauri_plugin_shell::{process::{CommandChild, CommandEvent}, ShellExt};
use tokio::sync::watch;

/// Connection details for the Go engine sidecar, handed to the frontend.
#[derive(Clone, Serialize, Deserialize)]
struct Engine {
    url: String,
    token: String,
}

#[derive(Deserialize)]
struct Handshake {
    port: u16,
    token: String,
}

struct EngineState {
    ready: watch::Receiver<Option<Engine>>,
    // Holding the child keeps its stdin open. The engine exits when stdin closes.
    _child: Mutex<CommandChild>,
}

/// Waits until the engine has printed its handshake, then returns how to reach it.
#[tauri::command]
async fn engine(state: tauri::State<'_, EngineState>) -> Result<Engine, String> {
    let mut rx = state.ready.clone();
    let engine = rx
        .wait_for(Option::is_some)
        .await
        .map_err(|_| "engine exited before it was ready".to_string())?;
    Ok(engine.clone().unwrap())
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_opener::init())
        .plugin(tauri_plugin_notification::init())
        .plugin(tauri_plugin_shell::init())
        .setup(|app| {
            // The engine keeps its database in the app data folder (~/Library/Application Support/dev.inboxed.app).
            // Debug builds get their own subfolder, so accounts added while developing never mix with a real install's.
            let debug = cfg!(debug_assertions);
            let mut data_dir = app.path().app_data_dir()?;
            if debug {
                data_dir.push("debug");
            }
            std::fs::create_dir_all(&data_dir)?;
            let engine = app
                .shell()
                .sidecar("inboxed-engine")?
                .env("INBOXED_DATA_DIR", &data_dir)
                // Development-only knobs, set explicitly so a variable exported in the shell can't reach the engine.
                .env("INBOXED_SEED", "") // demo mail is for `npm run dev:engine` (the browser) only
                .env("INBOXED_ORIGINS", if debug { "http://localhost:1420" } else { "" }) // the Vite dev server
                .env("INBOXED_PORT", "")
                .env("INBOXED_TOKEN", "")
                .env("INBOXED_FAKE_KEYCHAIN", "");
            let (mut events, child) = engine.spawn()?;
            let (tx, rx) = watch::channel(None);
            app.manage(EngineState { ready: rx, _child: Mutex::new(child) });

            tauri::async_runtime::spawn(async move {
                while let Some(event) = events.recv().await {
                    match event {
                        CommandEvent::Stdout(line) if tx.borrow().is_none() => {
                            if let Ok(h) = serde_json::from_slice::<Handshake>(&line) {
                                let url = format!("http://127.0.0.1:{}", h.port);
                                tx.send_replace(Some(Engine { url, token: h.token }));
                            }
                        }
                        CommandEvent::Stderr(line) => {
                            eprintln!("[engine] {}", String::from_utf8_lossy(&line).trim_end())
                        }
                        CommandEvent::Terminated(p) => {
                            eprintln!("[engine] exited: {:?}", p.code);
                            break;
                        }
                        _ => {}
                    }
                }
            });
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![engine])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
