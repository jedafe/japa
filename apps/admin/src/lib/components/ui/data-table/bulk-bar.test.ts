import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/svelte";
import BulkBar from "./bulk-bar.svelte";
import { Trash2 } from "lucide-svelte";

describe("BulkBar", () => {
  it("renders nothing when no ids selected", () => {
    const { container } = render(BulkBar, {
      selectedIds: new Set<string>(),
      actions: [],
      onclear: () => {},
    });
    expect(container.querySelector("[data-bulk-bar]")).toBeNull();
  });

  it("shows count and fires action onclick with the id set", async () => {
    const onDelete = vi.fn();
    const ids = new Set(["1", "2"]);
    render(BulkBar, {
      selectedIds: ids,
      actions: [
        { label: "Delete", icon: Trash2, onclick: onDelete, variant: "destructive" },
      ],
      onclear: () => {},
    });
    expect(screen.getByText(/2 selected/)).toBeTruthy();
    await fireEvent.click(screen.getByRole("button", { name: /delete/i }));
    expect(onDelete).toHaveBeenCalledWith(ids);
  });
});
