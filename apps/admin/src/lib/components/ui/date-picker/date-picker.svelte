<script lang="ts">
  import { DatePicker } from "bits-ui";
  import { CalendarDate, getLocalTimeZone, today } from "@internationalized/date";
  import { clsx, type ClassValue } from "clsx";
  import type { DateValue } from "@internationalized/date";
  
  let {
    value = $bindable(),
    placeholder = "Pick a date",
    disabled = false,
    class: className
  }: {
    value?: Date | undefined;
    placeholder?: string;
    disabled?: boolean;
    class?: ClassValue;
  } = $props();
  
  function formatDate(date: Date | undefined): string {
    if (!date) return placeholder;
    return date.toLocaleDateString("en-US", {
      year: "numeric",
      month: "short",
      day: "numeric"
    });
  }
  
  function toDateValue(date: Date | undefined): CalendarDate | undefined {
    if (!date) return undefined;
    return new CalendarDate(
      date.getFullYear(),
      date.getMonth() + 1,
      date.getDate()
    );
  }
  
  function fromDateValue(dateValue: DateValue | undefined): Date | undefined {
    if (!dateValue) return undefined;
    return new Date(dateValue.year, dateValue.month - 1, dateValue.day);
  }
  
  let internalValue = $state(toDateValue(value));
  
  function handleValueChange(newValue: DateValue | undefined) {
    internalValue = newValue as CalendarDate | undefined;
    value = fromDateValue(newValue);
  }
</script>

<DatePicker.Root
  bind:value={internalValue}
  onValueChange={handleValueChange}
  {disabled}
  locale="en-US"
  placeholder={today(getLocalTimeZone())}
>
  <DatePicker.Trigger
    class={clsx(
      "flex h-10 w-full items-center justify-between rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm ring-offset-white placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50",
      className
    )}
  >
    {#snippet child({ props })}
      <button {...props}>
        <DatePicker.Input class="flex h-full w-full items-center justify-between">
          {#snippet children({ segments })}
            {#each segments as segment}
              <DatePicker.Segment part={segment.part} class="px-0.5" />
            {/each}
          {/snippet}
        </DatePicker.Input>
      </button>
    {/snippet}
  </DatePicker.Trigger>
  
  <DatePicker.Portal>
    <DatePicker.Content
      class="z-50 rounded-lg border border-gray-200 bg-white p-3 shadow-lg data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95"
      sideOffset={4}
    >
      <DatePicker.Calendar class="rounded-lg">
        {#snippet children({ months, weekdays })}
          <DatePicker.Header class="flex items-center justify-between">
            <DatePicker.PrevButton class="h-7 w-7 rounded-lg p-0 opacity-50 hover:opacity-100" />
            <DatePicker.Heading class="text-sm font-medium" />
            <DatePicker.NextButton class="h-7 w-7 rounded-lg p-0 opacity-50 hover:opacity-100" />
          </DatePicker.Header>
          
          <DatePicker.Grid class="mt-2">
            <DatePicker.GridHead>
              <DatePicker.GridRow class="flex">
                {#each weekdays as day}
                  <DatePicker.HeadCell class="w-9 rounded-md text-xs font-medium text-gray-500">
                    {day}
                  </DatePicker.HeadCell>
                {/each}
              </DatePicker.GridRow>
            </DatePicker.GridHead>
            <DatePicker.GridBody>
              {#each months as month}
                {#each month.weeks as week}
                  <DatePicker.GridRow class="flex w-full">
                    {#each week as day}
                      <DatePicker.Cell
                        date={day}
                        month={month.value}
                        class="relative p-0 text-center text-sm focus-within:relative focus-within:z-20 [&:has([data-selected])]:rounded-lg [&:has([data-selected])]:bg-primary-100"
                      >
                        <DatePicker.Day
                          class="inline-flex h-9 w-9 items-center justify-center rounded-lg p-0 text-sm font-normal hover:bg-gray-100 focus:outline-none focus:ring-2 focus:ring-primary-500 data-[selected]:bg-primary-600 data-[selected]:text-white data-[disabled]:text-gray-300 data-[disabled]:pointer-events-none"
                        />
                      </DatePicker.Cell>
                    {/each}
                  </DatePicker.GridRow>
                {/each}
              {/each}
            </DatePicker.GridBody>
          </DatePicker.Grid>
        {/snippet}
      </DatePicker.Calendar>
    </DatePicker.Content>
  </DatePicker.Portal>
</DatePicker.Root>
