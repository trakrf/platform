interface DateRangeInputsProps {
  fromDate: string;
  toDate: string;
  onFromDateChange: (date: string) => void;
  onToDateChange: (date: string) => void;
}

export function DateRangeInputs({
  fromDate,
  toDate,
  onFromDateChange,
  onToDateChange,
}: DateRangeInputsProps) {
  return (
    <div className="flex items-end gap-3">
      <div className="flex flex-col gap-1">
        <label
          htmlFor="from-date"
          className="text-sm font-medium text-gray-700 dark:text-gray-300"
        >
          From
        </label>
        <input
          type="date"
          id="from-date"
          value={fromDate}
          onChange={(e) => onFromDateChange(e.target.value)}
          max={toDate}
          className="px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-blue-500"
        />
      </div>
      <div className="flex flex-col gap-1">
        <label
          htmlFor="to-date"
          className="text-sm font-medium text-gray-700 dark:text-gray-300"
        >
          To
        </label>
        <input
          type="date"
          id="to-date"
          value={toDate}
          onChange={(e) => onToDateChange(e.target.value)}
          min={fromDate}
          className="px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-blue-500"
        />
      </div>
    </div>
  );
}
