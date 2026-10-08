import ctypes
import logging
import os
import sys
import threading
import time

# Корректный вывод кириллицы и эмодзи в консоли Windows / в .exe
for _stream in (sys.stdout, sys.stderr):
    try:
        _stream.reconfigure(encoding="utf-8", errors="replace")
    except Exception:
        pass

# Лог рядом с .exe (или со скриптом)
BASE_DIR = os.path.dirname(sys.executable if getattr(sys, "frozen", False) else os.path.abspath(__file__))
LOG_PATH = os.path.join(BASE_DIR, "blocker.log")
logging.basicConfig(
    filename=LOG_PATH,
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(message)s",
    encoding="utf-8",
)
log = logging.getLogger("blocker")

# Комбинация для штатной остановки (Ctrl+C заблокирован, поэтому нужна своя)
STOP_HOTKEY = "ctrl+alt+shift+q"

HOTKEYS_TO_BLOCK = [
    "alt+tab",
    "alt+esc",
    "left windows",
    "right windows",
    "ctrl+c",
    "ctrl+v",
    "ctrl+insert",
    "shift+insert",
    "print screen",
    "ctrl+tab",
    "ctrl+shift+tab",
    "alt+f4",
    "ctrl+shift+esc",
]

stop_event = threading.Event()
blocked_count = 0


def is_admin() -> bool:
    if os.name != "nt":
        return hasattr(os, "geteuid") and os.geteuid() == 0
    try:
        return bool(ctypes.windll.shell32.IsUserAnAdmin())
    except Exception:
        return False


def on_blocked(hotkey: str) -> None:
    global blocked_count
    blocked_count += 1
    msg = f"Рұқсат етілмеген әрекет бұғатталды: {hotkey} (барлығы: {blocked_count})"
    print(f"⚠️ {msg}")
    log.warning(msg)


def register_hotkeys(keyboard) -> int:
    ok = 0
    for hk in HOTKEYS_TO_BLOCK:
        try:
            keyboard.add_hotkey(hk, lambda h=hk: on_blocked(h), suppress=True)
            ok += 1
        except Exception as exc:
            print(f"Скерту: {hk} бұғаттау мүмкін болмады: {exc}")
            log.error("Cannot block %s: %s", hk, exc)
    return ok


def main() -> int:
    print("========================================")
    print(" Argus AI - Local Proctoring Blocker    ")
    print("========================================")
    print("Бұғатталатын пернелер:", ", ".join(HOTKEYS_TO_BLOCK))
    print(f"Тоқтату: {STOP_HOTKEY}")
    print(f"Лог файлы: {LOG_PATH}")
    print("========================================\n")

    try:
        import keyboard
    except ImportError:
        print("Қате: 'keyboard' кітапханасы табылмады. pip install keyboard")
        log.critical("keyboard module is not installed")
        return 1

    if not is_admin():
        msg = "Әкімші құқығы жоқ — кейбір пернелер бұғатталмауы мүмкін (Run as administrator)."
        print(f"⚠️ {msg}")
        log.warning(msg)

    try:
        registered = register_hotkeys(keyboard)
        keyboard.add_hotkey(STOP_HOTKEY, stop_event.set)
    except Exception as exc:
        print(f"Қате: перне хуктарын орнату мүмкін болмады: {exc}")
        log.critical("Hook setup failed: %s", exc)
        return 2

    if registered == 0:
        print("Қате: бірде-бір перне бұғатталмады.")
        log.critical("No hotkeys registered")
        return 3

    print(f"Қорғаныс іске қосылды ({registered}/{len(HOTKEYS_TO_BLOCK)} тіркесім).\n")
    log.info("Protection started: %d/%d hotkeys", registered, len(HOTKEYS_TO_BLOCK))

    try:
        while not stop_event.is_set():
            time.sleep(0.5)
    except KeyboardInterrupt:
        pass
    finally:
        try:
            keyboard.unhook_all()
        except Exception as exc:
            log.error("unhook_all failed: %s", exc)
        print("\nҚорғаныс тоқтатылды.")
        log.info("Protection stopped. Blocked total: %d", blocked_count)
    return 0


if __name__ == "__main__":
    try:
        code = main()
    except Exception as exc:  # последняя страховка от вылета с трассировкой в консоль
        log.exception("Fatal error: %s", exc)
        print(f"Күтпеген қате: {exc}")
        code = 99
    if getattr(sys, "frozen", False):
        input("Шығу үшін Enter басыңыз...")
    sys.exit(code)

