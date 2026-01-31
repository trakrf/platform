interface DateRangeInputsProps {
  fromDate: string;
  toDate: string;
  onFromDateChange: (date: string) => void;
  onToDateChange: (date: string) => void;
  className?: string;
}

export function DateRangeInputs({
  fromDate,
  toDate,
  onFromDateChange,
  onToDateChange,
  className = '',
}: DateRangeInputsProps) {
  return (
    <div className={`flex items-end gap-3 ${className}`}>
      <div className="flex flex-col gap-1 flex-1 md:flex-none">
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
          className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-blue-500"
        />
      </div>
      <div className="flex flex-col gap-1 flex-1 md:flex-none">
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
          className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-800 text-gray-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-blue-500"
        />
      </div>
    </div>
  );
}
