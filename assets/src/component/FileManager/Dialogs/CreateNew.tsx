import { useTranslation } from "react-i18next";
import { DialogContent, Stack } from "@mui/material";
import { useAppDispatch, useAppSelector } from "../../../redux/hooks.ts";
import { ChangeEvent, useCallback, useEffect, useRef, useState } from "react";
import DraggableDialog from "../../Dialogs/DraggableDialog.tsx";
import { createNewDialogPromisePool } from "../../../redux/thunks/dialog.ts";
import { FilledTextField } from "../../Common/StyledComponents.tsx";
import { closeCreateNewDialog, CreateNewDialogType } from "../../../redux/globalStateSlice.ts";
import { submitCreateNew } from "../../../redux/thunks/file.ts";
import { FileType } from "../../../api/explorer.ts";

const CreateNew = () => {
  const { t } = useTranslation();
  const dispatch = useAppDispatch();

  const [name, setName] = useState("");
  const [urlName, setUrlName] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const formRef = useRef<HTMLFormElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const open = useAppSelector((state) => state.globalState.createNewDialogOpen);
  const promiseId = useAppSelector((state) => state.globalState.createNewPromiseId);
  const type = useAppSelector((state) => state.globalState.createNewDialogType);
  const defaultName = useAppSelector((state) => state.globalState.createNewDialogDefault);
  const fmIndex = useAppSelector((state) => state.globalState.createNewDialogFmIndex) ?? 0;

  useEffect(() => {
    if (open) {
      setName(type == CreateNewDialogType.folder ? "" : defaultName ?? "");
      setUrlName("");
      setError("");
    }
  }, [open]);

  const onClose = useCallback(() => {
    dispatch(closeCreateNewDialog());
    if (promiseId) {
      createNewDialogPromisePool[promiseId]?.reject("cancel");
    }
  }, [dispatch, promiseId]);

  const onAccept = useCallback(
    (e?: React.FormEvent<HTMLFormElement>) => {
      if (e) {
        e.preventDefault();
      }

      setLoading(true);
      dispatch(
        submitCreateNew(
          fmIndex,
          type == CreateNewDialogType.folder ? name.trim() || urlName : name,
          type == CreateNewDialogType.folder ? FileType.folder : FileType.file,
          type == CreateNewDialogType.folder ? urlName : undefined,
        ),
      )
        .then((f) => {
          if (promiseId) {
            createNewDialogPromisePool[promiseId]?.resolve(f);
          }
          dispatch(closeCreateNewDialog());
        })
        .catch((e) => {
          setError(e instanceof Error ? e.message : "创建失败，请检查名称是否重复。");
        })
        .finally(() => {
          setLoading(false);
        });
    },
    [promiseId, name, urlName, type, fmIndex],
  );

  const onOkClicked = useCallback(() => {
    if (formRef.current) {
      if (formRef.current.reportValidity()) {
        onAccept();
      }
    }
  }, [formRef, onAccept]);

  useEffect(() => {
    if (open) {
      const lastDot = name.lastIndexOf(".");
      setTimeout(
        () => inputRef.current && inputRef.current.setSelectionRange(0, lastDot > 0 ? lastDot : name.length),
        200,
      );
    }
  }, [open, inputRef.current]);

  const onNameChange = useCallback(
    (e: ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
      setName(e.target.value);
      if (error) {
        setError("");
      }
    },
    [dispatch, setName, error],
  );

  return (
    <DraggableDialog
      title={t(
        type == CreateNewDialogType.folder ? "application:fileManager.newFolder" : "application:fileManager.newFile",
      )}
      showActions
      loading={loading}
      showCancel
      onAccept={onOkClicked}
      dialogProps={{
        open: open ?? false,
        onClose: onClose,
        fullWidth: true,
        maxWidth: "sm",
        disableRestoreFocus: true,
      }}
    >
      <DialogContent>
        <Stack spacing={2}>
          <form ref={formRef} onSubmit={onAccept}>
            {type == CreateNewDialogType.folder && (
              <FilledTextField
                label="英文名（用于访问链接）"
                value={urlName}
                onChange={(e) => {
                  setUrlName(e.target.value);
                  setError("");
                }}
                autoFocus
                fullWidth
                required
                margin="dense"
                variant="filled"
                slotProps={{ htmlInput: { pattern: "[A-Za-z0-9_]+", maxLength: 64 } }}
                helperText="仅限英文字母、数字和下划线，最多 64 个字符；创建后固定。"
              />
            )}
            <FilledTextField
              inputRef={inputRef}
              variant="filled"
              autoFocus={type != CreateNewDialogType.folder}
              error={!!error}
              helperText={error}
              margin="dense"
              label={
                type == CreateNewDialogType.folder ? "中文名称（可选，用于显示）" : t("application:modals.fileName")
              }
              type="text"
              value={name}
              onChange={onNameChange}
              fullWidth
              required={type != CreateNewDialogType.folder}
            />
          </form>
        </Stack>
      </DialogContent>
    </DraggableDialog>
  );
};
export default CreateNew;
