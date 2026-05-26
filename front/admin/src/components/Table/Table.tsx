import styles from './Table.module.css'

interface Column {
  key: string
  label: string
}

interface TableProps {
  columns: Column[]
  data: Record<string, unknown>[]
}

export default function Table({ columns, data }: TableProps) {
  if (data.length === 0) {
    return <div className={styles.empty}>No data</div>
  }

  return (
    <table className={styles.table}>
      <thead>
        <tr>
          {columns.map((col) => (
            <th key={col.key}>{col.label}</th>
          ))}
        </tr>
      </thead>
      <tbody>
        {data.map((row, i) => (
          <tr key={i}>
            {columns.map((col) => (
              <td key={col.key}>{String(row[col.key] ?? '')}</td>
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  )
}
