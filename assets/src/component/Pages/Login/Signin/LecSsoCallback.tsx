import { Alert, Button, CircularProgress, Stack, Typography } from "@mui/material";
import { useEffect, useRef, useState } from "react";
import { LoginResponse } from "../../../../api/user";
import { useAppDispatch } from "../../../../redux/hooks";
import { refreshUserSession } from "../../../../redux/thunks/session";

export default function LecSsoCallback() {
  const dispatch = useAppDispatch();
  const started = useRef(false);
  const [error, setError] = useState("");

  useEffect(() => {
    // 授权码只能使用一次，避免组件重新渲染时重复兑换。
    if (started.current) return;
    started.current = true;
    const query = new URLSearchParams(window.location.search);
    void (async () => {
      try {
        const response = await fetch("/api/v4/session/lec-sso/complete", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ code: query.get("code"), state: query.get("state") }),
        });
        const result: { code: number; msg?: string; data: LoginResponse } = await response.json();
        if (!response.ok || result.code !== 0) throw new Error(result.msg || "统一登录失败，请重试。");
        window.history.replaceState(null, "", "/session/lec-sso/callback");
        dispatch(refreshUserSession(result.data, null));
      } catch (e) {
        setError(e instanceof Error ? e.message : "统一登录失败，请重试。");
      }
    })();
  }, [dispatch]);

  return (
    <Stack spacing={2} alignItems="center">
      {error ? (
        <>
          <Alert severity="error">{error}</Alert>
          <Button href="/session">重新登录</Button>
        </>
      ) : (
        <>
          <CircularProgress />
          <Typography>正在完成统一登录…</Typography>
        </>
      )}
    </Stack>
  );
}
