/*
Copyright 2017, 2019 the Velero contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package output

import (
	"fmt"
	"regexp"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/duration"
	"k8s.io/kubernetes/pkg/printers"

	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
)

var (
	backupColumns = []metav1.TableColumnDefinition{
		// name needs Type and Format defined for the decorator to identify it:
		// https://github.com/kubernetes/kubernetes/blob/v1.15.3/pkg/printers/tableprinter.go#L204
		{Name: "Name", Type: "string", Format: "name"},
		{Name: "Status"},
		{Name: "Created"},
		{Name: "Expires"},
		{Name: "Storage Location"},
		{Name: "Selector"},
	}
)

func printBackupList(list *velerov1api.BackupList, options printers.PrintOptions) ([]metav1.TableRow, error) {
	sortBackupsByPrefixAndTimestamp(list)
	rows := make([]metav1.TableRow, 0, len(list.Items))

	for i := range list.Items {
		r, err := printBackup(&list.Items[i], options)
		if err != nil {
			return nil, err
		}
		rows = append(rows, r...)
	}
	return rows, nil
}

// sort by default alphabetically, but if backups stem from a common schedule
// (detected by the presence of a 14-digit timestamp suffix), then within that
// group, sort by newest to oldest (i.e. prefix ASC, suffix DESC)
var timestampSuffix = regexp.MustCompile("-[0-9]{14}$")

func sortBackupsByPrefixAndTimestamp(list *velerov1api.BackupList) {

	sort.Slice(list.Items, func(i, j int) bool {
		iSuffixIndex := timestampSuffix.FindStringIndex(list.Items[i].Name)
		jSuffixIndex := timestampSuffix.FindStringIndex(list.Items[j].Name)

		// one/both don't have a timestamp suffix, so sort alphabetically
		if iSuffixIndex == nil || jSuffixIndex == nil {
			return list.Items[i].Name < list.Items[j].Name
		}

		// different prefixes, so sort alphabetically
		if list.Items[i].Name[0:iSuffixIndex[0]] != list.Items[j].Name[0:jSuffixIndex[0]] {
			return list.Items[i].Name < list.Items[j].Name
		}

		// same prefixes, so sort based on suffix (desc)
		return list.Items[i].Name[iSuffixIndex[0]:] >= list.Items[j].Name[jSuffixIndex[0]:]
	})
}

func printBackup(backup *velerov1api.Backup, options printers.PrintOptions) ([]metav1.TableRow, error) {
	row := metav1.TableRow{
		Object: runtime.RawExtension{Object: backup},
	}

	expiration := backup.Status.Expiration.Time
	if expiration.IsZero() && backup.Spec.TTL.Duration > 0 {
		expiration = backup.CreationTimestamp.Add(backup.Spec.TTL.Duration)
	}

	status := string(backup.Status.Phase)
	if status == "" {
		status = string(velerov1api.BackupPhaseNew)
	}
	if backup.DeletionTimestamp != nil && !backup.DeletionTimestamp.Time.IsZero() {
		status = "Deleting"
	}
	if status == string(velerov1api.BackupPhasePartiallyFailed) {
		if backup.Status.Errors == 1 {
			status = fmt.Sprintf("%s (1 error)", status)
		} else {
			status = fmt.Sprintf("%s (%d errors)", status, backup.Status.Errors)
		}

	}

	location := backup.Spec.StorageLocation

	row.Cells = append(row.Cells, backup.Name, status, backup.Status.StartTimestamp.Time, humanReadableTimeFromNow(expiration), location, metav1.FormatLabelSelector(backup.Spec.LabelSelector))

	return []metav1.TableRow{row}, nil
}

func humanReadableTimeFromNow(when time.Time) string {
	if when.IsZero() {
		return "n/a"
	}

	now := time.Now()
	switch {
	case when == now || when.After(now):
		return duration.ShortHumanDuration(when.Sub(now))
	default:
		return fmt.Sprintf("%s ago", duration.ShortHumanDuration(now.Sub(when)))
	}
}
