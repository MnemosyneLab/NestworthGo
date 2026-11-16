import { EntityIcon, type EntityIconKind } from "@/components/icons/EntityIcon";
import { Label } from "@/components/ui/label";
import { FilterableSelect } from "@/components/ui/filterable-select";

export type EntitySelectOption = { id: string; name: string; iconKey: string };

export function EntitySelect({ id, label, value, options, emptyLabel, kind, onChange, submissionProps }: {
  id: string;
  label: string;
  value: string;
  options: EntitySelectOption[];
  emptyLabel: string;
  kind: EntityIconKind;
  onChange: (value: string) => void;
  submissionProps?: { "data-submission-field": string; "aria-invalid": boolean; "aria-describedby"?: string };
}) {
  return <div className="flex flex-col gap-1.5">
    <Label htmlFor={id}>{label}</Label>
    <FilterableSelect
      {...submissionProps}
      id={id}
      aria-label={label}
      value={value}
      placeholder={emptyLabel}
      options={[
        { value: "", label: emptyLabel },
        ...options.map(option => ({
          value: option.id,
          label: option.name,
          icon: <EntityIcon iconKey={option.iconKey} kind={kind} />,
        })),
      ]}
      onValueChange={onChange}
    />
  </div>;
}
