import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/svelte";
import FilterPanel from "./filter-panel.svelte";

describe("FilterPanel", () => {
  it("renders an Autocomplete per filter definition", async () => {
    render(FilterPanel, {
      filters: [
        {
          key: "status",
          label: "Status",
          options: [{ value: "active", label: "Active" }],
        },
      ],
      value: {},
    });
    // Expand the accordion so the filter inputs mount.
    const trigger = await screen.findByText("Filters");
    await fireEvent.click(trigger);
    // Option filters render through the shared Autocomplete — the field is
    // identifiable by its placeholder text.
    await waitFor(() => {
      expect(screen.getByPlaceholderText("Status")).toBeTruthy();
    });
  });

  it("fires onclear when Clear is clicked", async () => {
    const onclear = vi.fn();
    render(FilterPanel, { filters: [], value: {}, onclear });
    // Expand the accordion so the Clear button becomes visible/queryable
    const trigger = await screen.findByText("Filters");
    await fireEvent.click(trigger);
    const clearBtn = await screen.findByRole("button", { name: /clear/i });
    await fireEvent.click(clearBtn);
    expect(onclear).toHaveBeenCalled();
  });
});
