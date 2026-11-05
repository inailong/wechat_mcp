#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
WeChat Windows Database Decryption Key Extractor
Automated extractor for wechat_mcp.
Retrieves the 64-character hex decryption key from running WeChat on Windows
and automatically saves it into %APPDATA%\\wechat_mcp\\config.json.
"""

import sys
import os
import json
import re
import subprocess
import platform

def print_banner():
    print("=" * 60)
    print("   WeChat MCP - Windows Key Extractor & Auto-Configurator   ")
    print("=" * 60)

def is_wechat_running():
    if platform.system() != "Windows":
        return True # Non-windows fallback
    try:
        out = subprocess.check_output('tasklist /FI "IMAGENAME eq WeChat.exe"', shell=True, text=True, errors="ignore")
        return "WeChat.exe" in out
    except Exception:
        return False

def install_pywxdump():
    print("[*] PyWxDump is required to safely scan WeChat memory.")
    print("[*] Installing pywxdump via pip...")
    try:
        subprocess.check_call([sys.executable, "-m", "pip", "install", "--upgrade", "pywxdump", "-i", "https://pypi.tuna.tsinghua.edu.cn/simple"])
        print("[+] pywxdump installed successfully.")
        return True
    except subprocess.CalledProcessError:
        try:
            subprocess.check_call([sys.executable, "-m", "pip", "install", "--upgrade", "pywxdump"])
            print("[+] pywxdump installed successfully.")
            return True
        except Exception as e:
            print(f"[-] Failed to install pywxdump: {e}")
            return False

def extract_key_via_pywxdump():
    try:
        import pywxdump
    except ImportError:
        if not install_pywxdump():
            return None, None
        try:
            import pywxdump
        except ImportError:
            print("[-] Unable to import pywxdump even after installation.")
            return None, None

    print("[*] Inspecting WeChat process memory...")

    # 1. Try python module API first
    try:
        from pywxdump.wx_info import get_wechat_db_key, get_wechat_info
        info_list = get_wechat_info()
        if info_list and isinstance(info_list, list) and len(info_list) > 0:
            info = info_list[0]
            key = info.get("key") or info.get("Key")
            wxid = info.get("wxid") or info.get("Wxid")
            if key and len(key) == 64:
                return key, wxid
    except Exception:
        pass

    # 2. Try CLI execution: python -m pywxdump bias --auto
    commands_to_try = [
        [sys.executable, "-m", "pywxdump", "bias", "--auto"],
        [sys.executable, "-m", "pywxdump", "info"],
    ]

    for cmd in commands_to_try:
        try:
            print(f"[*] Running: {' '.join(cmd)}")
            out = subprocess.check_output(cmd, text=True, errors="ignore")
            # Scan for 64-character hex key
            match = re.search(r'([a-fA-F0-9]{64})', out)
            if match:
                key = match.group(1).lower()
                # Try finding wxid
                wxid_match = re.search(r'wxid_([a-zA-Z0-9_-]+)', out)
                wxid = f"wxid_{wxid_match.group(1)}" if wxid_match else ""
                return key, wxid
        except Exception as e:
            continue

    return None, None

def save_config(key, wxid):
    if platform.system() == "Windows":
        appdata = os.environ.get("APPDATA", os.path.expanduser("~\\AppData\\Roaming"))
        cfg_dir = os.path.join(appdata, "wechat_mcp")
    else:
        cfg_dir = os.path.expanduser("~/.config/wechat_mcp")

    os.makedirs(cfg_dir, exist_ok=True)
    cfg_file = os.path.join(cfg_dir, "config.json")

    data = {}
    if os.path.isfile(cfg_file):
        try:
            with open(cfg_file, "r", encoding="utf-8") as f:
                data = json.load(f)
        except Exception:
            data = {}

    data["decryptKey"] = key
    if wxid:
        data["wxid"] = wxid

    with open(cfg_file, "w", encoding="utf-8") as f:
        json.dump(data, f, indent=2, ensure_ascii=False)

    print(f"[+] Key successfully configured at: {cfg_file}")
    return cfg_file

def main():
    print_banner()

    if platform.system() == "Windows":
        if not is_wechat_running():
            print("[!] Warning: WeChat.exe does not appear to be running.")
            print("    Please start WeChat and log in before running this script.")
            resp = input("    Have you logged in to WeChat now? (y/n): ").strip().lower()
            if resp != "y":
                print("[-] Aborted. Please start WeChat and run again.")
                sys.exit(1)

    key, wxid = extract_key_via_pywxdump()

    if not key:
        print("\n[-] Automated key extraction failed.")
        print("    You can still manually retrieve the key using one of the following methods:")
        print("    1. Run: pip install pywxdump; pywxdump bias --auto")
        print("    2. If using WeFlow, start WeFlow with HTTP API enabled (port 5031)")
        print("    3. Provide the key manually:")
        manual_key = input("    Enter 64-char Hex Key (or press Enter to exit): ").strip()
        if len(manual_key) == 64:
            key = manual_key.lower()
        else:
            print("[-] No valid key provided. Exiting.")
            sys.exit(1)

    print("\n" + "=" * 60)
    print(f"[+] Success! Extracted Database Key: {key}")
    if wxid:
        print(f"[+] Account WxID: {wxid}")
    print("=" * 60)

    cfg_file = save_config(key, wxid)

    # If wechat_mcp.exe exists, run diagnostics
    exe_candidates = [
        os.path.join(os.path.dirname(__file__), "..", "bin", "wechat_mcp.exe"),
        os.path.join(os.path.dirname(__file__), "..", "bin", "wechat_mcp"),
    ]
    for exe in exe_candidates:
        if os.path.isfile(exe):
            print("\n[*] Running wechat_mcp diagnostic doctor...")
            try:
                subprocess.call([exe, "--doctor"])
            except Exception as e:
                print(f"[!] Could not run diagnostics: {e}")
            break

if __name__ == "__main__":
    main()
