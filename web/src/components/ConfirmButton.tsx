import { useState } from "react";
import { useTranslation } from "react-i18next";

import { Button, type ButtonProps } from "@/components/ui/button";

/** A destructive action that asks once inline instead of opening a dialog. */
export function ConfirmButton({ onConfirm, children, question, ...props }: ButtonProps & { onConfirm: () => void; question: string }) {
  const { t } = useTranslation();
  const [asking, setAsking] = useState(false);
  if (!asking) {
    return (
      <Button {...props} onClick={() => setAsking(true)}>
        {children}
      </Button>
    );
  }
  return (
    <span className="inline-flex flex-wrap items-center gap-2 text-sm">
      <span>{question}</span>
      <Button
        size="sm"
        className="bg-red-600 text-white hover:bg-red-700"
        onClick={() => {
          setAsking(false);
          onConfirm();
        }}
      >
        {t("admin.confirm")}
      </Button>
      <Button size="sm" variant="ghost" onClick={() => setAsking(false)}>
        {t("admin.cancel")}
      </Button>
    </span>
  );
}
