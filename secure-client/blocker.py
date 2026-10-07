import keyboard
import time
import sys
import os

print("========================================")
print(" Argus AI - Local Proctoring Blocker    ")
print("========================================")
print("Бұл скрипт тестілеу кезінде келесі пернелерді бұғаттайды:")
print("- Alt + Tab")
print("- Windows пернесі (Left/Right)")
print("- Ctrl + C / Ctrl + V")
print("- Print Screen")
print("Тоқтату үшін терминалда: Ctrl + Pause/Break немесе терезесін жабыңыз.")
print("========================================\n")

# Бұғатталатын пернелер тіркесімдері
hotkeys_to_block = [
    'alt+tab',
    'left windows',
    'right windows',
    'ctrl+c',
    'ctrl+v',
    'print screen',
    'ctrl+tab',
    'ctrl+shift+tab',
    'alt+f4',       # терезені жабуға қарсы
    'ctrl+shift+esc' # task manager
]

def block_key(e):
    print(f"⚠️ Рұқсат етілмеген әрекет бұғатталды: {e.name}")
    return False

# Әрбір горячий клавиш үшін бұғаттауды орнату
for hk in hotkeys_to_block:
    try:
        # suppress=True пернені жүйеге жібермей ұстап қалады
        keyboard.add_hotkey(hk, lambda: print(f"⚠️ Рұқсат етілмеген әрекет бұғатталды!"), suppress=True)
    except Exception as e:
        print(f"Скерту: {hk} бұғаттау мүмкін болмады: {e}")

try:
    print("Қорғаныс іске қосылды. Студенттің жұмыс ортасы қорғалған.\n")
    # Скриптті шексіз жұмыс істету
    keyboard.wait()
except KeyboardInterrupt:
    print("\nҚорғаныс тоқтатылды.")
    sys.exit(0)
