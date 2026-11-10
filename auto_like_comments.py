# -*- coding: utf-8 -*-
"""auto_like_comments.py — 定时给小红书收到的新评论点赞(纯脚本,无需 AI)

流程:
1. GET  /api/v1/notifications/list?tab=mentions  拉取「评论和@」通知
2. 过滤 liked == false 且带 comment_id 的条目(本地日志二次去重)
3. POST /api/v1/notifications/like  逐条点赞(服务端对已赞评论自动跳过,幂等)
4. 结果追加写入 auto_like_log.json

用法: python auto_like_comments.py   (需 xiaohongshu-mcp 服务已在 18060 端口运行)
"""
import json
import os
import random
import sys
import time
from datetime import datetime

import requests

BASE = "http://127.0.0.1:18060"
TIMEOUT = 60
HERE = os.path.dirname(os.path.abspath(__file__))
LOG_FILE = os.path.join(HERE, "auto_like_log.json")
MAX_LIKES_PER_RUN = 20   # 单次最多点赞数,防风控
DELAY_RANGE = (3, 8)     # 两次点赞之间的随机间隔(秒)
MAX_ATTEMPTS = 3         # 同一条评论最多尝试次数,超过则放弃(如老帖评论定位不到)


def now_str():
    return datetime.now().strftime("%Y-%m-%d %H:%M:%S")


def load_log():
    if os.path.exists(LOG_FILE):
        try:
            with open(LOG_FILE, encoding="utf-8") as f:
                return json.load(f)
        except Exception:
            pass
    return []


def save_log(log):
    tmp = LOG_FILE + ".tmp"
    with open(tmp, "w", encoding="utf-8") as f:
        json.dump(log, f, ensure_ascii=False, indent=1)
    os.replace(tmp, LOG_FILE)


def main():
    log = load_log()
    done_ids = {e["comment_id"] for e in log if e.get("ok")}
    fail_counts = {}
    for e in log:
        if e.get("event") == "like" and not e.get("ok"):
            fail_counts[e["comment_id"]] = fail_counts.get(e["comment_id"], 0) + 1
    session = requests.Session()

    # 1. 拉取「评论和@」通知
    try:
        r = session.get(f"{BASE}/api/v1/notifications/list",
                        params={"tab": "mentions", "limit": 30}, timeout=TIMEOUT)
        r.raise_for_status()
        body = r.json()
        items = body.get("data", {}).get("data", {}).get("items", [])
    except Exception as e:
        print(f"[{now_str()}] 拉取通知失败(服务未启动或 cookies 失效?): {e}")
        log.append({"time": now_str(), "event": "fetch_failed", "error": str(e)})
        save_log(log)
        return 1

    # 2. 过滤待点赞条目
    pending = [it for it in items
               if it.get("comment_id") and it.get("liked") is False
               and it["comment_id"] not in done_ids
               and fail_counts.get(it["comment_id"], 0) < MAX_ATTEMPTS]
    skipped_old = [it for it in items
                   if it.get("comment_id") and it.get("liked") is False
                   and fail_counts.get(it["comment_id"], 0) >= MAX_ATTEMPTS]
    print(f"[{now_str()}] 通知 {len(items)} 条,待点赞 {len(pending)} 条"
          + (f",跳过 {len(skipped_old)} 条(失败超 {MAX_ATTEMPTS} 次)" if skipped_old else ""))

    ok = fail = 0
    for it in pending[:MAX_LIKES_PER_RUN]:
        cid = it["comment_id"]
        try:
            r = session.post(f"{BASE}/api/v1/notifications/like",
                             json={"comment_id": cid}, timeout=TIMEOUT)
            r.raise_for_status()
            result = r.json().get("data", {}).get("data", {})
            ok += 1
            nick = it.get("from", {}).get("nickname", "?")
            print(f"  + 已赞 {nick}: {str(it.get('comment_text', ''))[:30]} "
                  f"(liked={result.get('liked')}, skipped={result.get('skipped')})")
            log.append({"time": now_str(), "event": "like", "ok": True,
                        "comment_id": cid, "nickname": nick,
                        "text": it.get("comment_text"),
                        "feed_title": it.get("feed_title")})
        except Exception as e:
            fail += 1
            print(f"  x 失败 {cid}: {e}")
            log.append({"time": now_str(), "event": "like", "ok": False,
                        "comment_id": cid, "error": str(e)})
        time.sleep(random.uniform(*DELAY_RANGE))

    save_log(log)
    print(f"[{now_str()}] 完成: 成功 {ok},失败 {fail}")
    return 0 if fail == 0 else 1


if __name__ == "__main__":
    try:
        sys.stdout.reconfigure(encoding="utf-8", errors="replace")
    except Exception:
        pass
    sys.exit(main())
