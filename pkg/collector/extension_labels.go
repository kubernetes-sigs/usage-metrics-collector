// Copyright 2023 The Kubernetes Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package collector

import (
	"regexp"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/usage-metrics-collector/pkg/api/collectorcontrollerv1alpha1"
	"sigs.k8s.io/usage-metrics-collector/pkg/api/quotamanagementv1alpha1"
)

const MaxExtensionLabels = collectorcontrollerv1alpha1.MaxExtensionLabels

// extensionLabelsKeys defines the label keys for a MetricLabels
type extensionLabelsKeys struct {
	Label00 collectorcontrollerv1alpha1.LabelName

	Label01 collectorcontrollerv1alpha1.LabelName

	Label02 collectorcontrollerv1alpha1.LabelName

	Label03 collectorcontrollerv1alpha1.LabelName

	Label04 collectorcontrollerv1alpha1.LabelName

	Label05 collectorcontrollerv1alpha1.LabelName

	Label06 collectorcontrollerv1alpha1.LabelName

	Label07 collectorcontrollerv1alpha1.LabelName

	Label08 collectorcontrollerv1alpha1.LabelName

	Label09 collectorcontrollerv1alpha1.LabelName

	Label10 collectorcontrollerv1alpha1.LabelName

	Label11 collectorcontrollerv1alpha1.LabelName

	Label12 collectorcontrollerv1alpha1.LabelName

	Label13 collectorcontrollerv1alpha1.LabelName

	Label14 collectorcontrollerv1alpha1.LabelName

	Label15 collectorcontrollerv1alpha1.LabelName

	Label16 collectorcontrollerv1alpha1.LabelName

	Label17 collectorcontrollerv1alpha1.LabelName

	Label18 collectorcontrollerv1alpha1.LabelName

	Label19 collectorcontrollerv1alpha1.LabelName

	Label20 collectorcontrollerv1alpha1.LabelName

	Label21 collectorcontrollerv1alpha1.LabelName

	Label22 collectorcontrollerv1alpha1.LabelName

	Label23 collectorcontrollerv1alpha1.LabelName

	Label24 collectorcontrollerv1alpha1.LabelName

	Label25 collectorcontrollerv1alpha1.LabelName

	Label26 collectorcontrollerv1alpha1.LabelName

	Label27 collectorcontrollerv1alpha1.LabelName

	Label28 collectorcontrollerv1alpha1.LabelName

	Label29 collectorcontrollerv1alpha1.LabelName

	Label30 collectorcontrollerv1alpha1.LabelName

	Label31 collectorcontrollerv1alpha1.LabelName

	Label32 collectorcontrollerv1alpha1.LabelName

	Label33 collectorcontrollerv1alpha1.LabelName

	Label34 collectorcontrollerv1alpha1.LabelName

	Label35 collectorcontrollerv1alpha1.LabelName

	Label36 collectorcontrollerv1alpha1.LabelName

	Label37 collectorcontrollerv1alpha1.LabelName

	Label38 collectorcontrollerv1alpha1.LabelName

	Label39 collectorcontrollerv1alpha1.LabelName

	Label40 collectorcontrollerv1alpha1.LabelName

	Label41 collectorcontrollerv1alpha1.LabelName

	Label42 collectorcontrollerv1alpha1.LabelName

	Label43 collectorcontrollerv1alpha1.LabelName

	Label44 collectorcontrollerv1alpha1.LabelName

	Label45 collectorcontrollerv1alpha1.LabelName

	Label46 collectorcontrollerv1alpha1.LabelName

	Label47 collectorcontrollerv1alpha1.LabelName

	Label48 collectorcontrollerv1alpha1.LabelName

	Label49 collectorcontrollerv1alpha1.LabelName

	Label50 collectorcontrollerv1alpha1.LabelName

	Label51 collectorcontrollerv1alpha1.LabelName

	Label52 collectorcontrollerv1alpha1.LabelName

	Label53 collectorcontrollerv1alpha1.LabelName

	Label54 collectorcontrollerv1alpha1.LabelName

	Label55 collectorcontrollerv1alpha1.LabelName

	Label56 collectorcontrollerv1alpha1.LabelName

	Label57 collectorcontrollerv1alpha1.LabelName

	Label58 collectorcontrollerv1alpha1.LabelName

	Label59 collectorcontrollerv1alpha1.LabelName

	Label60 collectorcontrollerv1alpha1.LabelName

	Label61 collectorcontrollerv1alpha1.LabelName

	Label62 collectorcontrollerv1alpha1.LabelName

	Label63 collectorcontrollerv1alpha1.LabelName

	Label64 collectorcontrollerv1alpha1.LabelName

	Label65 collectorcontrollerv1alpha1.LabelName

	Label66 collectorcontrollerv1alpha1.LabelName

	Label67 collectorcontrollerv1alpha1.LabelName

	Label68 collectorcontrollerv1alpha1.LabelName

	Label69 collectorcontrollerv1alpha1.LabelName

	Label70 collectorcontrollerv1alpha1.LabelName

	Label71 collectorcontrollerv1alpha1.LabelName

	Label72 collectorcontrollerv1alpha1.LabelName

	Label73 collectorcontrollerv1alpha1.LabelName

	Label74 collectorcontrollerv1alpha1.LabelName

	Label75 collectorcontrollerv1alpha1.LabelName

	Label76 collectorcontrollerv1alpha1.LabelName

	Label77 collectorcontrollerv1alpha1.LabelName

	Label78 collectorcontrollerv1alpha1.LabelName

	Label79 collectorcontrollerv1alpha1.LabelName

	Label80 collectorcontrollerv1alpha1.LabelName

	Label81 collectorcontrollerv1alpha1.LabelName

	Label82 collectorcontrollerv1alpha1.LabelName

	Label83 collectorcontrollerv1alpha1.LabelName

	Label84 collectorcontrollerv1alpha1.LabelName

	Label85 collectorcontrollerv1alpha1.LabelName

	Label86 collectorcontrollerv1alpha1.LabelName

	Label87 collectorcontrollerv1alpha1.LabelName

	Label88 collectorcontrollerv1alpha1.LabelName

	Label89 collectorcontrollerv1alpha1.LabelName

	Label90 collectorcontrollerv1alpha1.LabelName

	Label91 collectorcontrollerv1alpha1.LabelName

	Label92 collectorcontrollerv1alpha1.LabelName

	Label93 collectorcontrollerv1alpha1.LabelName

	Label94 collectorcontrollerv1alpha1.LabelName

	Label95 collectorcontrollerv1alpha1.LabelName

	Label96 collectorcontrollerv1alpha1.LabelName

	Label97 collectorcontrollerv1alpha1.LabelName

	Label98 collectorcontrollerv1alpha1.LabelName

	Label99 collectorcontrollerv1alpha1.LabelName
}

func (l *extensionLabelsKeys) GetKey(index int) collectorcontrollerv1alpha1.LabelName {
	switch index {
	case 0:
		return l.Label00
	case 1:
		return l.Label01
	case 2:
		return l.Label02
	case 3:
		return l.Label03
	case 4:
		return l.Label04
	case 5:
		return l.Label05
	case 6:
		return l.Label06
	case 7:
		return l.Label07
	case 8:
		return l.Label08
	case 9:
		return l.Label09
	case 10:
		return l.Label10
	case 11:
		return l.Label11
	case 12:
		return l.Label12
	case 13:
		return l.Label13
	case 14:
		return l.Label14
	case 15:
		return l.Label15
	case 16:
		return l.Label16
	case 17:
		return l.Label17
	case 18:
		return l.Label18
	case 19:
		return l.Label19
	case 20:
		return l.Label20
	case 21:
		return l.Label21
	case 22:
		return l.Label22
	case 23:
		return l.Label23
	case 24:
		return l.Label24
	case 25:
		return l.Label25
	case 26:
		return l.Label26
	case 27:
		return l.Label27
	case 28:
		return l.Label28
	case 29:
		return l.Label29
	case 30:
		return l.Label30
	case 31:
		return l.Label31
	case 32:
		return l.Label32
	case 33:
		return l.Label33
	case 34:
		return l.Label34
	case 35:
		return l.Label35
	case 36:
		return l.Label36
	case 37:
		return l.Label37
	case 38:
		return l.Label38
	case 39:
		return l.Label39
	case 40:
		return l.Label40
	case 41:
		return l.Label41
	case 42:
		return l.Label42
	case 43:
		return l.Label43
	case 44:
		return l.Label44
	case 45:
		return l.Label45
	case 46:
		return l.Label46
	case 47:
		return l.Label47
	case 48:
		return l.Label48
	case 49:
		return l.Label49
	case 50:
		return l.Label50
	case 51:
		return l.Label51
	case 52:
		return l.Label52
	case 53:
		return l.Label53
	case 54:
		return l.Label54
	case 55:
		return l.Label55
	case 56:
		return l.Label56
	case 57:
		return l.Label57
	case 58:
		return l.Label58
	case 59:
		return l.Label59
	case 60:
		return l.Label60
	case 61:
		return l.Label61
	case 62:
		return l.Label62
	case 63:
		return l.Label63
	case 64:
		return l.Label64
	case 65:
		return l.Label65
	case 66:
		return l.Label66
	case 67:
		return l.Label67
	case 68:
		return l.Label68
	case 69:
		return l.Label69
	case 70:
		return l.Label70
	case 71:
		return l.Label71
	case 72:
		return l.Label72
	case 73:
		return l.Label73
	case 74:
		return l.Label74
	case 75:
		return l.Label75
	case 76:
		return l.Label76
	case 77:
		return l.Label77
	case 78:
		return l.Label78
	case 79:
		return l.Label79
	case 80:
		return l.Label80
	case 81:
		return l.Label81
	case 82:
		return l.Label82
	case 83:
		return l.Label83
	case 84:
		return l.Label84
	case 85:
		return l.Label85
	case 86:
		return l.Label86
	case 87:
		return l.Label87
	case 88:
		return l.Label88
	case 89:
		return l.Label89
	case 90:
		return l.Label90
	case 91:
		return l.Label91
	case 92:
		return l.Label92
	case 93:
		return l.Label93
	case 94:
		return l.Label94
	case 95:
		return l.Label95
	case 96:
		return l.Label96
	case 97:
		return l.Label97
	case 98:
		return l.Label98
	case 99:
		return l.Label99
	}
	return ""
}

func (l *extensionLabelsKeys) SetKey(name collectorcontrollerv1alpha1.LabelName, index collectorcontrollerv1alpha1.LabelId) {
	switch index {
	case 0:
		l.Label00 = name
	case 1:
		l.Label01 = name
	case 2:
		l.Label02 = name
	case 3:
		l.Label03 = name
	case 4:
		l.Label04 = name
	case 5:
		l.Label05 = name
	case 6:
		l.Label06 = name
	case 7:
		l.Label07 = name
	case 8:
		l.Label08 = name
	case 9:
		l.Label09 = name
	case 10:
		l.Label10 = name
	case 11:
		l.Label11 = name
	case 12:
		l.Label12 = name
	case 13:
		l.Label13 = name
	case 14:
		l.Label14 = name
	case 15:
		l.Label15 = name
	case 16:
		l.Label16 = name
	case 17:
		l.Label17 = name
	case 18:
		l.Label18 = name
	case 19:
		l.Label19 = name
	case 20:
		l.Label20 = name
	case 21:
		l.Label21 = name
	case 22:
		l.Label22 = name
	case 23:
		l.Label23 = name
	case 24:
		l.Label24 = name
	case 25:
		l.Label25 = name
	case 26:
		l.Label26 = name
	case 27:
		l.Label27 = name
	case 28:
		l.Label28 = name
	case 29:
		l.Label29 = name
	case 30:
		l.Label30 = name
	case 31:
		l.Label31 = name
	case 32:
		l.Label32 = name
	case 33:
		l.Label33 = name
	case 34:
		l.Label34 = name
	case 35:
		l.Label35 = name
	case 36:
		l.Label36 = name
	case 37:
		l.Label37 = name
	case 38:
		l.Label38 = name
	case 39:
		l.Label39 = name
	case 40:
		l.Label40 = name
	case 41:
		l.Label41 = name
	case 42:
		l.Label42 = name
	case 43:
		l.Label43 = name
	case 44:
		l.Label44 = name
	case 45:
		l.Label45 = name
	case 46:
		l.Label46 = name
	case 47:
		l.Label47 = name
	case 48:
		l.Label48 = name
	case 49:
		l.Label49 = name
	case 50:
		l.Label50 = name
	case 51:
		l.Label51 = name
	case 52:
		l.Label52 = name
	case 53:
		l.Label53 = name
	case 54:
		l.Label54 = name
	case 55:
		l.Label55 = name
	case 56:
		l.Label56 = name
	case 57:
		l.Label57 = name
	case 58:
		l.Label58 = name
	case 59:
		l.Label59 = name
	case 60:
		l.Label60 = name
	case 61:
		l.Label61 = name
	case 62:
		l.Label62 = name
	case 63:
		l.Label63 = name
	case 64:
		l.Label64 = name
	case 65:
		l.Label65 = name
	case 66:
		l.Label66 = name
	case 67:
		l.Label67 = name
	case 68:
		l.Label68 = name
	case 69:
		l.Label69 = name
	case 70:
		l.Label70 = name
	case 71:
		l.Label71 = name
	case 72:
		l.Label72 = name
	case 73:
		l.Label73 = name
	case 74:
		l.Label74 = name
	case 75:
		l.Label75 = name
	case 76:
		l.Label76 = name
	case 77:
		l.Label77 = name
	case 78:
		l.Label78 = name
	case 79:
		l.Label79 = name
	case 80:
		l.Label80 = name
	case 81:
		l.Label81 = name
	case 82:
		l.Label82 = name
	case 83:
		l.Label83 = name
	case 84:
		l.Label84 = name
	case 85:
		l.Label85 = name
	case 86:
		l.Label86 = name
	case 87:
		l.Label87 = name
	case 88:
		l.Label88 = name
	case 89:
		l.Label89 = name
	case 90:
		l.Label90 = name
	case 91:
		l.Label91 = name
	case 92:
		l.Label92 = name
	case 93:
		l.Label93 = name
	case 94:
		l.Label94 = name
	case 95:
		l.Label95 = name
	case 96:
		l.Label96 = name
	case 97:
		l.Label97 = name
	case 98:
		l.Label98 = name
	case 99:
		l.Label99 = name
	}
}

func (l *extensionLabelsKeys) GetIndex(name collectorcontrollerv1alpha1.LabelName) collectorcontrollerv1alpha1.LabelId {
	switch name {
	case l.Label00:
		return 0
	case l.Label01:
		return 1
	case l.Label02:
		return 2
	case l.Label03:
		return 3
	case l.Label04:
		return 4
	case l.Label05:
		return 5
	case l.Label06:
		return 6
	case l.Label07:
		return 7
	case l.Label08:
		return 8
	case l.Label09:
		return 9
	case l.Label10:
		return 10
	case l.Label11:
		return 11
	case l.Label12:
		return 12
	case l.Label13:
		return 13
	case l.Label14:
		return 14
	case l.Label15:
		return 15
	case l.Label16:
		return 16
	case l.Label17:
		return 17
	case l.Label18:
		return 18
	case l.Label19:
		return 19
	case l.Label20:
		return 20
	case l.Label21:
		return 21
	case l.Label22:
		return 22
	case l.Label23:
		return 23
	case l.Label24:
		return 24
	case l.Label25:
		return 25
	case l.Label26:
		return 26
	case l.Label27:
		return 27
	case l.Label28:
		return 28
	case l.Label29:
		return 29
	case l.Label30:
		return 30
	case l.Label31:
		return 31
	case l.Label32:
		return 32
	case l.Label33:
		return 33
	case l.Label34:
		return 34
	case l.Label35:
		return 35
	case l.Label36:
		return 36
	case l.Label37:
		return 37
	case l.Label38:
		return 38
	case l.Label39:
		return 39
	case l.Label40:
		return 40
	case l.Label41:
		return 41
	case l.Label42:
		return 42
	case l.Label43:
		return 43
	case l.Label44:
		return 44
	case l.Label45:
		return 45
	case l.Label46:
		return 46
	case l.Label47:
		return 47
	case l.Label48:
		return 48
	case l.Label49:
		return 49
	case l.Label50:
		return 50
	case l.Label51:
		return 51
	case l.Label52:
		return 52
	case l.Label53:
		return 53
	case l.Label54:
		return 54
	case l.Label55:
		return 55
	case l.Label56:
		return 56
	case l.Label57:
		return 57
	case l.Label58:
		return 58
	case l.Label59:
		return 59
	case l.Label60:
		return 60
	case l.Label61:
		return 61
	case l.Label62:
		return 62
	case l.Label63:
		return 63
	case l.Label64:
		return 64
	case l.Label65:
		return 65
	case l.Label66:
		return 66
	case l.Label67:
		return 67
	case l.Label68:
		return 68
	case l.Label69:
		return 69
	case l.Label70:
		return 70
	case l.Label71:
		return 71
	case l.Label72:
		return 72
	case l.Label73:
		return 73
	case l.Label74:
		return 74
	case l.Label75:
		return 75
	case l.Label76:
		return 76
	case l.Label77:
		return 77
	case l.Label78:
		return 78
	case l.Label79:
		return 79
	case l.Label80:
		return 80
	case l.Label81:
		return 81
	case l.Label82:
		return 82
	case l.Label83:
		return 83
	case l.Label84:
		return 84
	case l.Label85:
		return 85
	case l.Label86:
		return 86
	case l.Label87:
		return 87
	case l.Label88:
		return 88
	case l.Label89:
		return 89
	case l.Label90:
		return 90
	case l.Label91:
		return 91
	case l.Label92:
		return 92
	case l.Label93:
		return 93
	case l.Label94:
		return 94
	case l.Label95:
		return 95
	case l.Label96:
		return 96
	case l.Label97:
		return 97
	case l.Label98:
		return 98
	case l.Label99:
		return 99
	}
	return -1
}

// extensionLabelsValues contains the label values for a time series
type extensionLabelsValues struct {
	Label00 string

	Label01 string

	Label02 string

	Label03 string

	Label04 string

	Label05 string

	Label06 string

	Label07 string

	Label08 string

	Label09 string

	Label10 string

	Label11 string

	Label12 string

	Label13 string

	Label14 string

	Label15 string

	Label16 string

	Label17 string

	Label18 string

	Label19 string

	Label20 string

	Label21 string

	Label22 string

	Label23 string

	Label24 string

	Label25 string

	Label26 string

	Label27 string

	Label28 string

	Label29 string

	Label30 string

	Label31 string

	Label32 string

	Label33 string

	Label34 string

	Label35 string

	Label36 string

	Label37 string

	Label38 string

	Label39 string

	Label40 string

	Label41 string

	Label42 string

	Label43 string

	Label44 string

	Label45 string

	Label46 string

	Label47 string

	Label48 string

	Label49 string

	Label50 string

	Label51 string

	Label52 string

	Label53 string

	Label54 string

	Label55 string

	Label56 string

	Label57 string

	Label58 string

	Label59 string

	Label60 string

	Label61 string

	Label62 string

	Label63 string

	Label64 string

	Label65 string

	Label66 string

	Label67 string

	Label68 string

	Label69 string

	Label70 string

	Label71 string

	Label72 string

	Label73 string

	Label74 string

	Label75 string

	Label76 string

	Label77 string

	Label78 string

	Label79 string

	Label80 string

	Label81 string

	Label82 string

	Label83 string

	Label84 string

	Label85 string

	Label86 string

	Label87 string

	Label88 string

	Label89 string

	Label90 string

	Label91 string

	Label92 string

	Label93 string

	Label94 string

	Label95 string

	Label96 string

	Label97 string

	Label98 string

	Label99 string
}

func (l *extensionLabelsValues) GetValueForName(names extensionLabelsKeys, name collectorcontrollerv1alpha1.LabelName) string {
	return l.GetValue(names.GetIndex(name))
}

func (l *extensionLabelsValues) GetValue(index collectorcontrollerv1alpha1.LabelId) string {
	switch index {
	case 0:
		return l.Label00
	case 1:
		return l.Label01
	case 2:
		return l.Label02
	case 3:
		return l.Label03
	case 4:
		return l.Label04
	case 5:
		return l.Label05
	case 6:
		return l.Label06
	case 7:
		return l.Label07
	case 8:
		return l.Label08
	case 9:
		return l.Label09
	case 10:
		return l.Label10
	case 11:
		return l.Label11
	case 12:
		return l.Label12
	case 13:
		return l.Label13
	case 14:
		return l.Label14
	case 15:
		return l.Label15
	case 16:
		return l.Label16
	case 17:
		return l.Label17
	case 18:
		return l.Label18
	case 19:
		return l.Label19
	case 20:
		return l.Label20
	case 21:
		return l.Label21
	case 22:
		return l.Label22
	case 23:
		return l.Label23
	case 24:
		return l.Label24
	case 25:
		return l.Label25
	case 26:
		return l.Label26
	case 27:
		return l.Label27
	case 28:
		return l.Label28
	case 29:
		return l.Label29
	case 30:
		return l.Label30
	case 31:
		return l.Label31
	case 32:
		return l.Label32
	case 33:
		return l.Label33
	case 34:
		return l.Label34
	case 35:
		return l.Label35
	case 36:
		return l.Label36
	case 37:
		return l.Label37
	case 38:
		return l.Label38
	case 39:
		return l.Label39
	case 40:
		return l.Label40
	case 41:
		return l.Label41
	case 42:
		return l.Label42
	case 43:
		return l.Label43
	case 44:
		return l.Label44
	case 45:
		return l.Label45
	case 46:
		return l.Label46
	case 47:
		return l.Label47
	case 48:
		return l.Label48
	case 49:
		return l.Label49
	case 50:
		return l.Label50
	case 51:
		return l.Label51
	case 52:
		return l.Label52
	case 53:
		return l.Label53
	case 54:
		return l.Label54
	case 55:
		return l.Label55
	case 56:
		return l.Label56
	case 57:
		return l.Label57
	case 58:
		return l.Label58
	case 59:
		return l.Label59
	case 60:
		return l.Label60
	case 61:
		return l.Label61
	case 62:
		return l.Label62
	case 63:
		return l.Label63
	case 64:
		return l.Label64
	case 65:
		return l.Label65
	case 66:
		return l.Label66
	case 67:
		return l.Label67
	case 68:
		return l.Label68
	case 69:
		return l.Label69
	case 70:
		return l.Label70
	case 71:
		return l.Label71
	case 72:
		return l.Label72
	case 73:
		return l.Label73
	case 74:
		return l.Label74
	case 75:
		return l.Label75
	case 76:
		return l.Label76
	case 77:
		return l.Label77
	case 78:
		return l.Label78
	case 79:
		return l.Label79
	case 80:
		return l.Label80
	case 81:
		return l.Label81
	case 82:
		return l.Label82
	case 83:
		return l.Label83
	case 84:
		return l.Label84
	case 85:
		return l.Label85
	case 86:
		return l.Label86
	case 87:
		return l.Label87
	case 88:
		return l.Label88
	case 89:
		return l.Label89
	case 90:
		return l.Label90
	case 91:
		return l.Label91
	case 92:
		return l.Label92
	case 93:
		return l.Label93
	case 94:
		return l.Label94
	case 95:
		return l.Label95
	case 96:
		return l.Label96
	case 97:
		return l.Label97
	case 98:
		return l.Label98
	case 99:
		return l.Label99

	}
	return ""

}

func (l *extensionLabelsValues) SetValue(index collectorcontrollerv1alpha1.LabelId, value string) {
	switch index {
	case 0:
		l.Label00 = value
	case 1:
		l.Label01 = value
	case 2:
		l.Label02 = value
	case 3:
		l.Label03 = value
	case 4:
		l.Label04 = value
	case 5:
		l.Label05 = value
	case 6:
		l.Label06 = value
	case 7:
		l.Label07 = value
	case 8:
		l.Label08 = value
	case 9:
		l.Label09 = value
	case 10:
		l.Label10 = value
	case 11:
		l.Label11 = value
	case 12:
		l.Label12 = value
	case 13:
		l.Label13 = value
	case 14:
		l.Label14 = value
	case 15:
		l.Label15 = value
	case 16:
		l.Label16 = value
	case 17:
		l.Label17 = value
	case 18:
		l.Label18 = value
	case 19:
		l.Label19 = value
	case 20:
		l.Label20 = value
	case 21:
		l.Label21 = value
	case 22:
		l.Label22 = value
	case 23:
		l.Label23 = value
	case 24:
		l.Label24 = value
	case 25:
		l.Label25 = value
	case 26:
		l.Label26 = value
	case 27:
		l.Label27 = value
	case 28:
		l.Label28 = value
	case 29:
		l.Label29 = value
	case 30:
		l.Label30 = value
	case 31:
		l.Label31 = value
	case 32:
		l.Label32 = value
	case 33:
		l.Label33 = value
	case 34:
		l.Label34 = value
	case 35:
		l.Label35 = value
	case 36:
		l.Label36 = value
	case 37:
		l.Label37 = value
	case 38:
		l.Label38 = value
	case 39:
		l.Label39 = value
	case 40:
		l.Label40 = value
	case 41:
		l.Label41 = value
	case 42:
		l.Label42 = value
	case 43:
		l.Label43 = value
	case 44:
		l.Label44 = value
	case 45:
		l.Label45 = value
	case 46:
		l.Label46 = value
	case 47:
		l.Label47 = value
	case 48:
		l.Label48 = value
	case 49:
		l.Label49 = value
	case 50:
		l.Label50 = value
	case 51:
		l.Label51 = value
	case 52:
		l.Label52 = value
	case 53:
		l.Label53 = value
	case 54:
		l.Label54 = value
	case 55:
		l.Label55 = value
	case 56:
		l.Label56 = value
	case 57:
		l.Label57 = value
	case 58:
		l.Label58 = value
	case 59:
		l.Label59 = value
	case 60:
		l.Label60 = value
	case 61:
		l.Label61 = value
	case 62:
		l.Label62 = value
	case 63:
		l.Label63 = value
	case 64:
		l.Label64 = value
	case 65:
		l.Label65 = value
	case 66:
		l.Label66 = value
	case 67:
		l.Label67 = value
	case 68:
		l.Label68 = value
	case 69:
		l.Label69 = value
	case 70:
		l.Label70 = value
	case 71:
		l.Label71 = value
	case 72:
		l.Label72 = value
	case 73:
		l.Label73 = value
	case 74:
		l.Label74 = value
	case 75:
		l.Label75 = value
	case 76:
		l.Label76 = value
	case 77:
		l.Label77 = value
	case 78:
		l.Label78 = value
	case 79:
		l.Label79 = value
	case 80:
		l.Label80 = value
	case 81:
		l.Label81 = value
	case 82:
		l.Label82 = value
	case 83:
		l.Label83 = value
	case 84:
		l.Label84 = value
	case 85:
		l.Label85 = value
	case 86:
		l.Label86 = value
	case 87:
		l.Label87 = value
	case 88:
		l.Label88 = value
	case 89:
		l.Label89 = value
	case 90:
		l.Label90 = value
	case 91:
		l.Label91 = value
	case 92:
		l.Label92 = value
	case 93:
		l.Label93 = value
	case 94:
		l.Label94 = value
	case 95:
		l.Label95 = value
	case 96:
		l.Label96 = value
	case 97:
		l.Label97 = value
	case 98:
		l.Label98 = value
	case 99:
		l.Label99 = value
	}
}

func (l *extensionLabelsValues) SetValueForName(names extensionLabelsKeys, name collectorcontrollerv1alpha1.LabelName, value string) {
	l.SetValue(names.GetIndex(name), value)
}

// extensionLabelsMask applies a mask to the Labels
type extensionLabelsMask struct {
	keys extensionLabelsKeys

	Label00 bool

	Label01 bool

	Label02 bool

	Label03 bool

	Label04 bool

	Label05 bool

	Label06 bool

	Label07 bool

	Label08 bool

	Label09 bool

	Label10 bool

	Label11 bool

	Label12 bool

	Label13 bool

	Label14 bool

	Label15 bool

	Label16 bool

	Label17 bool

	Label18 bool

	Label19 bool

	Label20 bool

	Label21 bool

	Label22 bool

	Label23 bool

	Label24 bool

	Label25 bool

	Label26 bool

	Label27 bool

	Label28 bool

	Label29 bool

	Label30 bool

	Label31 bool

	Label32 bool

	Label33 bool

	Label34 bool

	Label35 bool

	Label36 bool

	Label37 bool

	Label38 bool

	Label39 bool

	Label40 bool

	Label41 bool

	Label42 bool

	Label43 bool

	Label44 bool

	Label45 bool

	Label46 bool

	Label47 bool

	Label48 bool

	Label49 bool

	Label50 bool

	Label51 bool

	Label52 bool

	Label53 bool

	Label54 bool

	Label55 bool

	Label56 bool

	Label57 bool

	Label58 bool

	Label59 bool

	Label60 bool

	Label61 bool

	Label62 bool

	Label63 bool

	Label64 bool

	Label65 bool

	Label66 bool

	Label67 bool

	Label68 bool

	Label69 bool

	Label70 bool

	Label71 bool

	Label72 bool

	Label73 bool

	Label74 bool

	Label75 bool

	Label76 bool

	Label77 bool

	Label78 bool

	Label79 bool

	Label80 bool

	Label81 bool

	Label82 bool

	Label83 bool

	Label84 bool

	Label85 bool

	Label86 bool

	Label87 bool

	Label88 bool

	Label89 bool

	Label90 bool

	Label91 bool

	Label92 bool

	Label93 bool

	Label94 bool

	Label95 bool

	Label96 bool

	Label97 bool

	Label98 bool

	Label99 bool
}

func (m extensionLabelsMask) Mask(l extensionLabelsValues) extensionLabelsValues {
	if !m.Label00 {
		l.Label00 = ""
	}
	if !m.Label01 {
		l.Label01 = ""
	}
	if !m.Label02 {
		l.Label02 = ""
	}
	if !m.Label03 {
		l.Label03 = ""
	}
	if !m.Label04 {
		l.Label04 = ""
	}
	if !m.Label05 {
		l.Label05 = ""
	}
	if !m.Label06 {
		l.Label06 = ""
	}
	if !m.Label07 {
		l.Label07 = ""
	}
	if !m.Label08 {
		l.Label08 = ""
	}
	if !m.Label09 {
		l.Label09 = ""
	}
	if !m.Label10 {
		l.Label10 = ""
	}
	if !m.Label11 {
		l.Label11 = ""
	}
	if !m.Label12 {
		l.Label12 = ""
	}
	if !m.Label13 {
		l.Label13 = ""
	}
	if !m.Label14 {
		l.Label14 = ""
	}
	if !m.Label15 {
		l.Label15 = ""
	}
	if !m.Label16 {
		l.Label16 = ""
	}
	if !m.Label17 {
		l.Label17 = ""
	}
	if !m.Label18 {
		l.Label18 = ""
	}
	if !m.Label19 {
		l.Label19 = ""
	}
	if !m.Label20 {
		l.Label20 = ""
	}
	if !m.Label21 {
		l.Label21 = ""
	}
	if !m.Label22 {
		l.Label22 = ""
	}
	if !m.Label23 {
		l.Label23 = ""
	}
	if !m.Label24 {
		l.Label24 = ""
	}
	if !m.Label25 {
		l.Label25 = ""
	}
	if !m.Label26 {
		l.Label26 = ""
	}
	if !m.Label27 {
		l.Label27 = ""
	}
	if !m.Label28 {
		l.Label28 = ""
	}
	if !m.Label29 {
		l.Label29 = ""
	}
	if !m.Label30 {
		l.Label30 = ""
	}
	if !m.Label31 {
		l.Label31 = ""
	}
	if !m.Label32 {
		l.Label32 = ""
	}
	if !m.Label33 {
		l.Label33 = ""
	}
	if !m.Label34 {
		l.Label34 = ""
	}
	if !m.Label35 {
		l.Label35 = ""
	}
	if !m.Label36 {
		l.Label36 = ""
	}
	if !m.Label37 {
		l.Label37 = ""
	}
	if !m.Label38 {
		l.Label38 = ""
	}
	if !m.Label39 {
		l.Label39 = ""
	}
	if !m.Label40 {
		l.Label40 = ""
	}
	if !m.Label41 {
		l.Label41 = ""
	}
	if !m.Label42 {
		l.Label42 = ""
	}
	if !m.Label43 {
		l.Label43 = ""
	}
	if !m.Label44 {
		l.Label44 = ""
	}
	if !m.Label45 {
		l.Label45 = ""
	}
	if !m.Label46 {
		l.Label46 = ""
	}
	if !m.Label47 {
		l.Label47 = ""
	}
	if !m.Label48 {
		l.Label48 = ""
	}
	if !m.Label49 {
		l.Label49 = ""
	}
	if !m.Label50 {
		l.Label50 = ""
	}
	if !m.Label51 {
		l.Label51 = ""
	}
	if !m.Label52 {
		l.Label52 = ""
	}
	if !m.Label53 {
		l.Label53 = ""
	}
	if !m.Label54 {
		l.Label54 = ""
	}
	if !m.Label55 {
		l.Label55 = ""
	}
	if !m.Label56 {
		l.Label56 = ""
	}
	if !m.Label57 {
		l.Label57 = ""
	}
	if !m.Label58 {
		l.Label58 = ""
	}
	if !m.Label59 {
		l.Label59 = ""
	}
	if !m.Label60 {
		l.Label60 = ""
	}
	if !m.Label61 {
		l.Label61 = ""
	}
	if !m.Label62 {
		l.Label62 = ""
	}
	if !m.Label63 {
		l.Label63 = ""
	}
	if !m.Label64 {
		l.Label64 = ""
	}
	if !m.Label65 {
		l.Label65 = ""
	}
	if !m.Label66 {
		l.Label66 = ""
	}
	if !m.Label67 {
		l.Label67 = ""
	}
	if !m.Label68 {
		l.Label68 = ""
	}
	if !m.Label69 {
		l.Label69 = ""
	}
	if !m.Label70 {
		l.Label70 = ""
	}
	if !m.Label71 {
		l.Label71 = ""
	}
	if !m.Label72 {
		l.Label72 = ""
	}
	if !m.Label73 {
		l.Label73 = ""
	}
	if !m.Label74 {
		l.Label74 = ""
	}
	if !m.Label75 {
		l.Label75 = ""
	}
	if !m.Label76 {
		l.Label76 = ""
	}
	if !m.Label77 {
		l.Label77 = ""
	}
	if !m.Label78 {
		l.Label78 = ""
	}
	if !m.Label79 {
		l.Label79 = ""
	}
	if !m.Label80 {
		l.Label80 = ""
	}
	if !m.Label81 {
		l.Label81 = ""
	}
	if !m.Label82 {
		l.Label82 = ""
	}
	if !m.Label83 {
		l.Label83 = ""
	}
	if !m.Label84 {
		l.Label84 = ""
	}
	if !m.Label85 {
		l.Label85 = ""
	}
	if !m.Label86 {
		l.Label86 = ""
	}
	if !m.Label87 {
		l.Label87 = ""
	}
	if !m.Label88 {
		l.Label88 = ""
	}
	if !m.Label89 {
		l.Label89 = ""
	}
	if !m.Label90 {
		l.Label90 = ""
	}
	if !m.Label91 {
		l.Label91 = ""
	}
	if !m.Label92 {
		l.Label92 = ""
	}
	if !m.Label93 {
		l.Label93 = ""
	}
	if !m.Label94 {
		l.Label94 = ""
	}
	if !m.Label95 {
		l.Label95 = ""
	}
	if !m.Label96 {
		l.Label96 = ""
	}
	if !m.Label97 {
		l.Label97 = ""
	}
	if !m.Label98 {
		l.Label98 = ""
	}
	if !m.Label99 {
		l.Label99 = ""
	}
	return l
}

func (mask extensionLabelsMask) GetLabelNames() []string {
	result := make([]string, 0, collectorcontrollerv1alpha1.MaxExtensionLabels)
	if mask.Label00 {
		result = append(result, string(mask.keys.Label00))
	}
	if mask.Label01 {
		result = append(result, string(mask.keys.Label01))
	}
	if mask.Label02 {
		result = append(result, string(mask.keys.Label02))
	}
	if mask.Label03 {
		result = append(result, string(mask.keys.Label03))
	}
	if mask.Label04 {
		result = append(result, string(mask.keys.Label04))
	}
	if mask.Label05 {
		result = append(result, string(mask.keys.Label05))
	}
	if mask.Label06 {
		result = append(result, string(mask.keys.Label06))
	}
	if mask.Label07 {
		result = append(result, string(mask.keys.Label07))
	}
	if mask.Label08 {
		result = append(result, string(mask.keys.Label08))
	}
	if mask.Label09 {
		result = append(result, string(mask.keys.Label09))
	}
	if mask.Label10 {
		result = append(result, string(mask.keys.Label10))
	}
	if mask.Label11 {
		result = append(result, string(mask.keys.Label11))
	}
	if mask.Label12 {
		result = append(result, string(mask.keys.Label12))
	}
	if mask.Label13 {
		result = append(result, string(mask.keys.Label13))
	}
	if mask.Label14 {
		result = append(result, string(mask.keys.Label14))
	}
	if mask.Label15 {
		result = append(result, string(mask.keys.Label15))
	}
	if mask.Label16 {
		result = append(result, string(mask.keys.Label16))
	}
	if mask.Label17 {
		result = append(result, string(mask.keys.Label17))
	}
	if mask.Label18 {
		result = append(result, string(mask.keys.Label18))
	}
	if mask.Label19 {
		result = append(result, string(mask.keys.Label19))
	}
	if mask.Label20 {
		result = append(result, string(mask.keys.Label20))
	}
	if mask.Label21 {
		result = append(result, string(mask.keys.Label21))
	}
	if mask.Label22 {
		result = append(result, string(mask.keys.Label22))
	}
	if mask.Label23 {
		result = append(result, string(mask.keys.Label23))
	}
	if mask.Label24 {
		result = append(result, string(mask.keys.Label24))
	}
	if mask.Label25 {
		result = append(result, string(mask.keys.Label25))
	}
	if mask.Label26 {
		result = append(result, string(mask.keys.Label26))
	}
	if mask.Label27 {
		result = append(result, string(mask.keys.Label27))
	}
	if mask.Label28 {
		result = append(result, string(mask.keys.Label28))
	}
	if mask.Label29 {
		result = append(result, string(mask.keys.Label29))
	}
	if mask.Label30 {
		result = append(result, string(mask.keys.Label30))
	}
	if mask.Label31 {
		result = append(result, string(mask.keys.Label31))
	}
	if mask.Label32 {
		result = append(result, string(mask.keys.Label32))
	}
	if mask.Label33 {
		result = append(result, string(mask.keys.Label33))
	}
	if mask.Label34 {
		result = append(result, string(mask.keys.Label34))
	}
	if mask.Label35 {
		result = append(result, string(mask.keys.Label35))
	}
	if mask.Label36 {
		result = append(result, string(mask.keys.Label36))
	}
	if mask.Label37 {
		result = append(result, string(mask.keys.Label37))
	}
	if mask.Label38 {
		result = append(result, string(mask.keys.Label38))
	}
	if mask.Label39 {
		result = append(result, string(mask.keys.Label39))
	}
	if mask.Label40 {
		result = append(result, string(mask.keys.Label40))
	}
	if mask.Label41 {
		result = append(result, string(mask.keys.Label41))
	}
	if mask.Label42 {
		result = append(result, string(mask.keys.Label42))
	}
	if mask.Label43 {
		result = append(result, string(mask.keys.Label43))
	}
	if mask.Label44 {
		result = append(result, string(mask.keys.Label44))
	}
	if mask.Label45 {
		result = append(result, string(mask.keys.Label45))
	}
	if mask.Label46 {
		result = append(result, string(mask.keys.Label46))
	}
	if mask.Label47 {
		result = append(result, string(mask.keys.Label47))
	}
	if mask.Label48 {
		result = append(result, string(mask.keys.Label48))
	}
	if mask.Label49 {
		result = append(result, string(mask.keys.Label49))
	}
	if mask.Label50 {
		result = append(result, string(mask.keys.Label50))
	}
	if mask.Label51 {
		result = append(result, string(mask.keys.Label51))
	}
	if mask.Label52 {
		result = append(result, string(mask.keys.Label52))
	}
	if mask.Label53 {
		result = append(result, string(mask.keys.Label53))
	}
	if mask.Label54 {
		result = append(result, string(mask.keys.Label54))
	}
	if mask.Label55 {
		result = append(result, string(mask.keys.Label55))
	}
	if mask.Label56 {
		result = append(result, string(mask.keys.Label56))
	}
	if mask.Label57 {
		result = append(result, string(mask.keys.Label57))
	}
	if mask.Label58 {
		result = append(result, string(mask.keys.Label58))
	}
	if mask.Label59 {
		result = append(result, string(mask.keys.Label59))
	}
	if mask.Label60 {
		result = append(result, string(mask.keys.Label60))
	}
	if mask.Label61 {
		result = append(result, string(mask.keys.Label61))
	}
	if mask.Label62 {
		result = append(result, string(mask.keys.Label62))
	}
	if mask.Label63 {
		result = append(result, string(mask.keys.Label63))
	}
	if mask.Label64 {
		result = append(result, string(mask.keys.Label64))
	}
	if mask.Label65 {
		result = append(result, string(mask.keys.Label65))
	}
	if mask.Label66 {
		result = append(result, string(mask.keys.Label66))
	}
	if mask.Label67 {
		result = append(result, string(mask.keys.Label67))
	}
	if mask.Label68 {
		result = append(result, string(mask.keys.Label68))
	}
	if mask.Label69 {
		result = append(result, string(mask.keys.Label69))
	}
	if mask.Label70 {
		result = append(result, string(mask.keys.Label70))
	}
	if mask.Label71 {
		result = append(result, string(mask.keys.Label71))
	}
	if mask.Label72 {
		result = append(result, string(mask.keys.Label72))
	}
	if mask.Label73 {
		result = append(result, string(mask.keys.Label73))
	}
	if mask.Label74 {
		result = append(result, string(mask.keys.Label74))
	}
	if mask.Label75 {
		result = append(result, string(mask.keys.Label75))
	}
	if mask.Label76 {
		result = append(result, string(mask.keys.Label76))
	}
	if mask.Label77 {
		result = append(result, string(mask.keys.Label77))
	}
	if mask.Label78 {
		result = append(result, string(mask.keys.Label78))
	}
	if mask.Label79 {
		result = append(result, string(mask.keys.Label79))
	}
	if mask.Label80 {
		result = append(result, string(mask.keys.Label80))
	}
	if mask.Label81 {
		result = append(result, string(mask.keys.Label81))
	}
	if mask.Label82 {
		result = append(result, string(mask.keys.Label82))
	}
	if mask.Label83 {
		result = append(result, string(mask.keys.Label83))
	}
	if mask.Label84 {
		result = append(result, string(mask.keys.Label84))
	}
	if mask.Label85 {
		result = append(result, string(mask.keys.Label85))
	}
	if mask.Label86 {
		result = append(result, string(mask.keys.Label86))
	}
	if mask.Label87 {
		result = append(result, string(mask.keys.Label87))
	}
	if mask.Label88 {
		result = append(result, string(mask.keys.Label88))
	}
	if mask.Label89 {
		result = append(result, string(mask.keys.Label89))
	}
	if mask.Label90 {
		result = append(result, string(mask.keys.Label90))
	}
	if mask.Label91 {
		result = append(result, string(mask.keys.Label91))
	}
	if mask.Label92 {
		result = append(result, string(mask.keys.Label92))
	}
	if mask.Label93 {
		result = append(result, string(mask.keys.Label93))
	}
	if mask.Label94 {
		result = append(result, string(mask.keys.Label94))
	}
	if mask.Label95 {
		result = append(result, string(mask.keys.Label95))
	}
	if mask.Label96 {
		result = append(result, string(mask.keys.Label96))
	}
	if mask.Label97 {
		result = append(result, string(mask.keys.Label97))
	}
	if mask.Label98 {
		result = append(result, string(mask.keys.Label98))
	}
	if mask.Label99 {
		result = append(result, string(mask.keys.Label99))
	}

	return result
}

func (mask extensionLabelsMask) GetLabelValues(m extensionLabelsValues) []string {
	result := make([]string, 0, collectorcontrollerv1alpha1.MaxExtensionLabels)
	if mask.Label00 {
		result = append(result, m.Label00)
	}
	if mask.Label01 {
		result = append(result, m.Label01)
	}
	if mask.Label02 {
		result = append(result, m.Label02)
	}
	if mask.Label03 {
		result = append(result, m.Label03)
	}
	if mask.Label04 {
		result = append(result, m.Label04)
	}
	if mask.Label05 {
		result = append(result, m.Label05)
	}
	if mask.Label06 {
		result = append(result, m.Label06)
	}
	if mask.Label07 {
		result = append(result, m.Label07)
	}
	if mask.Label08 {
		result = append(result, m.Label08)
	}
	if mask.Label09 {
		result = append(result, m.Label09)
	}
	if mask.Label10 {
		result = append(result, m.Label10)
	}
	if mask.Label11 {
		result = append(result, m.Label11)
	}
	if mask.Label12 {
		result = append(result, m.Label12)
	}
	if mask.Label13 {
		result = append(result, m.Label13)
	}
	if mask.Label14 {
		result = append(result, m.Label14)
	}
	if mask.Label15 {
		result = append(result, m.Label15)
	}
	if mask.Label16 {
		result = append(result, m.Label16)
	}
	if mask.Label17 {
		result = append(result, m.Label17)
	}
	if mask.Label18 {
		result = append(result, m.Label18)
	}
	if mask.Label19 {
		result = append(result, m.Label19)
	}
	if mask.Label20 {
		result = append(result, m.Label20)
	}
	if mask.Label21 {
		result = append(result, m.Label21)
	}
	if mask.Label22 {
		result = append(result, m.Label22)
	}
	if mask.Label23 {
		result = append(result, m.Label23)
	}
	if mask.Label24 {
		result = append(result, m.Label24)
	}
	if mask.Label25 {
		result = append(result, m.Label25)
	}
	if mask.Label26 {
		result = append(result, m.Label26)
	}
	if mask.Label27 {
		result = append(result, m.Label27)
	}
	if mask.Label28 {
		result = append(result, m.Label28)
	}
	if mask.Label29 {
		result = append(result, m.Label29)
	}
	if mask.Label30 {
		result = append(result, m.Label30)
	}
	if mask.Label31 {
		result = append(result, m.Label31)
	}
	if mask.Label32 {
		result = append(result, m.Label32)
	}
	if mask.Label33 {
		result = append(result, m.Label33)
	}
	if mask.Label34 {
		result = append(result, m.Label34)
	}
	if mask.Label35 {
		result = append(result, m.Label35)
	}
	if mask.Label36 {
		result = append(result, m.Label36)
	}
	if mask.Label37 {
		result = append(result, m.Label37)
	}
	if mask.Label38 {
		result = append(result, m.Label38)
	}
	if mask.Label39 {
		result = append(result, m.Label39)
	}
	if mask.Label40 {
		result = append(result, m.Label40)
	}
	if mask.Label41 {
		result = append(result, m.Label41)
	}
	if mask.Label42 {
		result = append(result, m.Label42)
	}
	if mask.Label43 {
		result = append(result, m.Label43)
	}
	if mask.Label44 {
		result = append(result, m.Label44)
	}
	if mask.Label45 {
		result = append(result, m.Label45)
	}
	if mask.Label46 {
		result = append(result, m.Label46)
	}
	if mask.Label47 {
		result = append(result, m.Label47)
	}
	if mask.Label48 {
		result = append(result, m.Label48)
	}
	if mask.Label49 {
		result = append(result, m.Label49)
	}
	if mask.Label50 {
		result = append(result, m.Label50)
	}
	if mask.Label51 {
		result = append(result, m.Label51)
	}
	if mask.Label52 {
		result = append(result, m.Label52)
	}
	if mask.Label53 {
		result = append(result, m.Label53)
	}
	if mask.Label54 {
		result = append(result, m.Label54)
	}
	if mask.Label55 {
		result = append(result, m.Label55)
	}
	if mask.Label56 {
		result = append(result, m.Label56)
	}
	if mask.Label57 {
		result = append(result, m.Label57)
	}
	if mask.Label58 {
		result = append(result, m.Label58)
	}
	if mask.Label59 {
		result = append(result, m.Label59)
	}
	if mask.Label60 {
		result = append(result, m.Label60)
	}
	if mask.Label61 {
		result = append(result, m.Label61)
	}
	if mask.Label62 {
		result = append(result, m.Label62)
	}
	if mask.Label63 {
		result = append(result, m.Label63)
	}
	if mask.Label64 {
		result = append(result, m.Label64)
	}
	if mask.Label65 {
		result = append(result, m.Label65)
	}
	if mask.Label66 {
		result = append(result, m.Label66)
	}
	if mask.Label67 {
		result = append(result, m.Label67)
	}
	if mask.Label68 {
		result = append(result, m.Label68)
	}
	if mask.Label69 {
		result = append(result, m.Label69)
	}
	if mask.Label70 {
		result = append(result, m.Label70)
	}
	if mask.Label71 {
		result = append(result, m.Label71)
	}
	if mask.Label72 {
		result = append(result, m.Label72)
	}
	if mask.Label73 {
		result = append(result, m.Label73)
	}
	if mask.Label74 {
		result = append(result, m.Label74)
	}
	if mask.Label75 {
		result = append(result, m.Label75)
	}
	if mask.Label76 {
		result = append(result, m.Label76)
	}
	if mask.Label77 {
		result = append(result, m.Label77)
	}
	if mask.Label78 {
		result = append(result, m.Label78)
	}
	if mask.Label79 {
		result = append(result, m.Label79)
	}
	if mask.Label80 {
		result = append(result, m.Label80)
	}
	if mask.Label81 {
		result = append(result, m.Label81)
	}
	if mask.Label82 {
		result = append(result, m.Label82)
	}
	if mask.Label83 {
		result = append(result, m.Label83)
	}
	if mask.Label84 {
		result = append(result, m.Label84)
	}
	if mask.Label85 {
		result = append(result, m.Label85)
	}
	if mask.Label86 {
		result = append(result, m.Label86)
	}
	if mask.Label87 {
		result = append(result, m.Label87)
	}
	if mask.Label88 {
		result = append(result, m.Label88)
	}
	if mask.Label89 {
		result = append(result, m.Label89)
	}
	if mask.Label90 {
		result = append(result, m.Label90)
	}
	if mask.Label91 {
		result = append(result, m.Label91)
	}
	if mask.Label92 {
		result = append(result, m.Label92)
	}
	if mask.Label93 {
		result = append(result, m.Label93)
	}
	if mask.Label94 {
		result = append(result, m.Label94)
	}
	if mask.Label95 {
		result = append(result, m.Label95)
	}
	if mask.Label96 {
		result = append(result, m.Label96)
	}
	if mask.Label97 {
		result = append(result, m.Label97)
	}
	if mask.Label98 {
		result = append(result, m.Label98)
	}
	if mask.Label99 {
		result = append(result, m.Label99)
	}
	return result
}

// getOverrideExtensionLabelValues overrides extension masks
func (mask extensionLabelsMask) getOverrideExtensionLabelValues(m extensionLabelsValues,
	overrides map[string]string) extensionLabelsValues {
	f := func(labelName, labelValue string) string {
		if val, ok := overrides[labelName]; ok {
			return val
		}
		return labelValue
	}
	s := m
	s.Label00 = f(string(mask.keys.Label00), m.Label00)
	s.Label01 = f(string(mask.keys.Label01), m.Label01)
	s.Label02 = f(string(mask.keys.Label02), m.Label02)
	s.Label03 = f(string(mask.keys.Label03), m.Label03)
	s.Label04 = f(string(mask.keys.Label04), m.Label04)
	s.Label05 = f(string(mask.keys.Label05), m.Label05)
	s.Label06 = f(string(mask.keys.Label06), m.Label06)
	s.Label07 = f(string(mask.keys.Label07), m.Label07)
	s.Label08 = f(string(mask.keys.Label08), m.Label08)
	s.Label09 = f(string(mask.keys.Label09), m.Label09)
	s.Label10 = f(string(mask.keys.Label10), m.Label10)
	s.Label11 = f(string(mask.keys.Label11), m.Label11)
	s.Label12 = f(string(mask.keys.Label12), m.Label12)
	s.Label13 = f(string(mask.keys.Label13), m.Label13)
	s.Label14 = f(string(mask.keys.Label14), m.Label14)
	s.Label15 = f(string(mask.keys.Label15), m.Label15)
	s.Label16 = f(string(mask.keys.Label16), m.Label16)
	s.Label17 = f(string(mask.keys.Label17), m.Label17)
	s.Label18 = f(string(mask.keys.Label18), m.Label18)
	s.Label19 = f(string(mask.keys.Label19), m.Label19)
	s.Label20 = f(string(mask.keys.Label20), m.Label20)
	s.Label21 = f(string(mask.keys.Label21), m.Label21)
	s.Label22 = f(string(mask.keys.Label22), m.Label22)
	s.Label23 = f(string(mask.keys.Label23), m.Label23)
	s.Label24 = f(string(mask.keys.Label24), m.Label24)
	s.Label25 = f(string(mask.keys.Label25), m.Label25)
	s.Label26 = f(string(mask.keys.Label26), m.Label26)
	s.Label27 = f(string(mask.keys.Label27), m.Label27)
	s.Label28 = f(string(mask.keys.Label28), m.Label28)
	s.Label29 = f(string(mask.keys.Label29), m.Label29)
	s.Label30 = f(string(mask.keys.Label30), m.Label30)
	s.Label31 = f(string(mask.keys.Label31), m.Label31)
	s.Label32 = f(string(mask.keys.Label32), m.Label32)
	s.Label33 = f(string(mask.keys.Label33), m.Label33)
	s.Label34 = f(string(mask.keys.Label34), m.Label34)
	s.Label35 = f(string(mask.keys.Label35), m.Label35)
	s.Label36 = f(string(mask.keys.Label36), m.Label36)
	s.Label37 = f(string(mask.keys.Label37), m.Label37)
	s.Label38 = f(string(mask.keys.Label38), m.Label38)
	s.Label39 = f(string(mask.keys.Label39), m.Label39)
	s.Label40 = f(string(mask.keys.Label40), m.Label40)
	s.Label41 = f(string(mask.keys.Label41), m.Label41)
	s.Label42 = f(string(mask.keys.Label42), m.Label42)
	s.Label43 = f(string(mask.keys.Label43), m.Label43)
	s.Label44 = f(string(mask.keys.Label44), m.Label44)
	s.Label45 = f(string(mask.keys.Label45), m.Label45)
	s.Label46 = f(string(mask.keys.Label46), m.Label46)
	s.Label47 = f(string(mask.keys.Label47), m.Label47)
	s.Label48 = f(string(mask.keys.Label48), m.Label48)
	s.Label49 = f(string(mask.keys.Label49), m.Label49)
	s.Label50 = f(string(mask.keys.Label50), m.Label50)
	s.Label51 = f(string(mask.keys.Label51), m.Label51)
	s.Label52 = f(string(mask.keys.Label52), m.Label52)
	s.Label53 = f(string(mask.keys.Label53), m.Label53)
	s.Label54 = f(string(mask.keys.Label54), m.Label54)
	s.Label55 = f(string(mask.keys.Label55), m.Label55)
	s.Label56 = f(string(mask.keys.Label56), m.Label56)
	s.Label57 = f(string(mask.keys.Label57), m.Label57)
	s.Label58 = f(string(mask.keys.Label58), m.Label58)
	s.Label59 = f(string(mask.keys.Label59), m.Label59)
	s.Label60 = f(string(mask.keys.Label60), m.Label60)
	s.Label61 = f(string(mask.keys.Label61), m.Label61)
	s.Label62 = f(string(mask.keys.Label62), m.Label62)
	s.Label63 = f(string(mask.keys.Label63), m.Label63)
	s.Label64 = f(string(mask.keys.Label64), m.Label64)
	s.Label65 = f(string(mask.keys.Label65), m.Label65)
	s.Label66 = f(string(mask.keys.Label66), m.Label66)
	s.Label67 = f(string(mask.keys.Label67), m.Label67)
	s.Label68 = f(string(mask.keys.Label68), m.Label68)
	s.Label69 = f(string(mask.keys.Label69), m.Label69)
	s.Label70 = f(string(mask.keys.Label70), m.Label70)
	s.Label71 = f(string(mask.keys.Label71), m.Label71)
	s.Label72 = f(string(mask.keys.Label72), m.Label72)
	s.Label73 = f(string(mask.keys.Label73), m.Label73)
	s.Label74 = f(string(mask.keys.Label74), m.Label74)
	s.Label75 = f(string(mask.keys.Label75), m.Label75)
	s.Label76 = f(string(mask.keys.Label76), m.Label76)
	s.Label77 = f(string(mask.keys.Label77), m.Label77)
	s.Label78 = f(string(mask.keys.Label78), m.Label78)
	s.Label79 = f(string(mask.keys.Label79), m.Label79)
	s.Label80 = f(string(mask.keys.Label80), m.Label80)
	s.Label81 = f(string(mask.keys.Label81), m.Label81)
	s.Label82 = f(string(mask.keys.Label82), m.Label82)
	s.Label83 = f(string(mask.keys.Label83), m.Label83)
	s.Label84 = f(string(mask.keys.Label84), m.Label84)
	s.Label85 = f(string(mask.keys.Label85), m.Label85)
	s.Label86 = f(string(mask.keys.Label86), m.Label86)
	s.Label87 = f(string(mask.keys.Label87), m.Label87)
	s.Label88 = f(string(mask.keys.Label88), m.Label88)
	s.Label89 = f(string(mask.keys.Label89), m.Label89)
	s.Label90 = f(string(mask.keys.Label90), m.Label90)
	s.Label91 = f(string(mask.keys.Label91), m.Label91)
	s.Label92 = f(string(mask.keys.Label92), m.Label92)
	s.Label93 = f(string(mask.keys.Label93), m.Label93)
	s.Label94 = f(string(mask.keys.Label94), m.Label94)
	s.Label95 = f(string(mask.keys.Label95), m.Label95)
	s.Label96 = f(string(mask.keys.Label96), m.Label96)
	s.Label97 = f(string(mask.keys.Label97), m.Label97)
	s.Label98 = f(string(mask.keys.Label98), m.Label98)
	s.Label99 = f(string(mask.keys.Label99), m.Label99)
	return s
}

type extensionLabler struct {
	Extensions collectorcontrollerv1alpha1.Extensions
	SideCar    []*collectorcontrollerv1alpha1.SideCarConfig
}

func (l extensionLabler) GetLabelNames() extensionLabelsKeys {
	var keys extensionLabelsKeys
	for _, v := range l.Extensions.Pods {
		keys.SetKey(v.LabelName, v.ID)
	}
	for _, v := range l.Extensions.Namespaces {
		keys.SetKey(v.LabelName, v.ID)
	}
	for _, v := range l.Extensions.Quota {
		keys.SetKey(v.LabelName, v.ID)
	}
	for _, v := range l.Extensions.Nodes {
		keys.SetKey(v.LabelName, v.ID)
	}
	return keys
}

func (l extensionLabler) SetLabelsForMetadata(
	labels *extensionLabelsValues,
	ext []collectorcontrollerv1alpha1.ExtensionLabel, meta *metav1.ObjectMeta) {
	for _, v := range ext {
		func() {
			// Set the metric label from the metadata.labels if a match is found
			if v.LabelKey != "" && meta.Labels[string(v.LabelKey)] != "" {
				labels.SetValue(v.ID, meta.Labels[string(v.LabelKey)])
				return
			}
			for _, e := range v.LabelKeys {
				if e != "" && meta.Labels[string(e)] != "" {
					labels.SetValue(v.ID, meta.Labels[string(e)])
					return
				}
			}
			// Set the metric label from the metadata.annotations if a match is found
			if v.AnnotationKey != "" && meta.Annotations[string(v.AnnotationKey)] != "" {
				labels.SetValue(v.ID, meta.Annotations[string(v.AnnotationKey)])
				return
			}
			for _, e := range v.AnnotationKeys {
				if e != "" && meta.Annotations[string(e)] != "" {
					labels.SetValue(v.ID, meta.Annotations[string(e)])
					return
				}
			}
			// Set the metric label to a default value
			if v.Value != "" {
				labels.SetValue(v.ID, v.Value)
				return
			}
		}()
	}
}

// SetLabelsForPod pulls label values off of the passed in Kubernetes objects
func (l extensionLabler) SetLabelsForPod(
	labels *extensionLabelsValues, pod *corev1.Pod, w workload,
	node *corev1.Node, namespace *corev1.Namespace) {

	if pod != nil {
		l.SetLabelsForMetadata(labels, l.Extensions.Pods, &pod.ObjectMeta)
		l.SetLabelsForResourceEntries(labels, &pod.Spec)
	}

	l.SetLabelsForQuota(labels, nil, nil, namespace)
	l.SetLabelsForNode(labels, node)
}

func (l extensionLabler) SetLabelsForPersistentVolume(labels *extensionLabelsValues,
	pv *corev1.PersistentVolume, _ *corev1.PersistentVolumeClaim, node *corev1.Node) {
	if pv != nil {
		l.SetLabelsForMetadata(labels, l.Extensions.PVs, &pv.ObjectMeta)
	}
	l.SetLabelsForNode(labels, node)
}

func (l extensionLabler) SetLabelsForPersistentVolumeClaim(
	labels *extensionLabelsValues,
	pvc *corev1.PersistentVolumeClaim, pv *corev1.PersistentVolume, namespace *corev1.Namespace,
	pod *corev1.Pod, w workload, node *corev1.Node) {
	if pvc != nil {
		l.SetLabelsForMetadata(labels, l.Extensions.PVCs, &pvc.ObjectMeta)
	}
	l.SetLabelsForPersistentVolume(labels, pv, pvc, node)
	l.SetLabelsForPod(labels, pod, w, node, namespace)
}

func (l extensionLabler) SetLabelsForNode(labels *extensionLabelsValues, node *corev1.Node) {
	l.SetLabelsForSideCars(labels)
	if node == nil {
		return
	}
	l.SetLabelsForMetadata(labels, l.Extensions.Nodes, &node.ObjectMeta)

	// get the labels for the taints
	for _, v := range l.Extensions.NodeTaints {
		labels.SetValue(v.ID, getLabelValueForNodeTaint(v, node))
	}
}

func (l extensionLabler) SetLabelsForQuota(labels *extensionLabelsValues,
	quota *corev1.ResourceQuota, rqd *quotamanagementv1alpha1.ResourceQuotaDescriptor, namespace *corev1.Namespace) {
	l.SetLabelsForNamespace(labels, namespace)
	if quota == nil {
		return
	}
	l.SetLabelsForMetadata(labels, l.Extensions.Quota, &quota.ObjectMeta)
}

func (l extensionLabler) SetLabelsForNamespace(labels *extensionLabelsValues, namespace *corev1.Namespace) {
	l.SetLabelsForSideCars(labels)
	if namespace == nil {
		return
	}
	l.SetLabelsForMetadata(labels, l.Extensions.Namespaces, &namespace.ObjectMeta)
}

func (l extensionLabler) SetLabelsForSideCars(labels *extensionLabelsValues) {
	for _, sc := range l.SideCar {
		for _, scl := range sc.Labels {
			// add these static labels
			labels.SetValue(scl.ID, scl.Value)
		}
	}
}

func (l extensionLabler) SetLabelsForResourceEntries(labels *extensionLabelsValues, podSpec *corev1.PodSpec) {
	if podSpec == nil {
		return
	}
	// get the labels for the resource entries
	for _, v := range l.Extensions.ResourceEntries {
		if labelValue := getLabelValueForResourceEntry(v, podSpec); labelValue != "" {
			labels.SetValue(v.ID, labelValue)
		}
	}
}

// IsMatch returns true if value meets all of the TaintRequirements
func isMatchNodeTaintRequirements(t collectorcontrollerv1alpha1.NodeTaintRequirements, value string) bool {
	for _, r := range t {
		if r.NodeTaintOperator == collectorcontrollerv1alpha1.NodeTaintOperatorOpIn {
			for _, v := range r.Values {
				if v == value {
					// matches -- meets "In"
					return true
				}
			}
			// no matches -- fails "In"
			return false
		}
		if r.NodeTaintOperator == collectorcontrollerv1alpha1.NodeTaintOperatorOpNotIn {
			for _, v := range r.Values {
				if v == value {
					// matches -- fails "NotIn"
					return false
				}
			}
			// no matches -- meets "NotIn"
			return true
		}
	}
	// no requirements
	return true
}

// IsMatch returns true if taint meets all of the NodeTaint requirements
func isMatchNodeTaint(t collectorcontrollerv1alpha1.NodeTaint, taint *corev1.Taint) bool {
	if !isMatchNodeTaintRequirements(t.TaintKeys, taint.Key) {
		return false
	}
	if !isMatchNodeTaintRequirements(t.TaintValues, taint.Value) {

		return false
	}
	if !isMatchNodeTaintRequirements(t.TaintEffects, string(taint.Effect)) {
		return false
	}
	return true
}

// GetLabelValue returns the label value for the node and true if the node
// matches the requirements for the NodeTaint
func getLabelValueForNodeTaint(t collectorcontrollerv1alpha1.NodeTaint, node *corev1.Node) string {
	for i := range node.Spec.Taints {
		if isMatchNodeTaint(t, &node.Spec.Taints[i]) {
			if t.LabelValue != "" {
				// match -- return the hard coded label value
				return t.LabelValue
			}
			return node.Spec.Taints[i].Value
		}
	}
	return t.LabelNegativeValue
}

// getLabelValueForResourceEntry returns the label value for the resource entries in the pod,
// in case it returns the empty string the label shhould be omitted.
func getLabelValueForResourceEntry(re collectorcontrollerv1alpha1.ResourceEntry, podSpec *corev1.PodSpec) string {
	if re.Key == "" && re.MatchKey == "" {
		return ""
	}

	var keyRE, valRE *regexp.Regexp
	var exactMatches, regexpMatches map[string]map[string]map[string]string

	if re.MatchKey != "" {
		// we ignore regexp compile errors as those are already validated
		keyRE, _ = regexp.Compile(re.MatchKey)
	}
	if re.MatchValue != "" {
		// we ignore regexp compile errors as those are already validated
		valRE, _ = regexp.Compile(re.MatchValue)
	}

	// collect matches

	if re.Key != "" {
		exactMatches = make(map[string]map[string]map[string]string)
		for _, ctr := range append(podSpec.InitContainers, podSpec.Containers...) {
			if v, ok := ctr.Resources.Requests[corev1.ResourceName(re.Key)]; ok {
				if exactMatches[ctr.Name] == nil {
					exactMatches[ctr.Name] = make(map[string]map[string]string)
					exactMatches[ctr.Name]["requests"] = make(map[string]string)
				}
				exactMatches[ctr.Name]["requests"][re.Key] = v.String()
			}
			if v, ok := ctr.Resources.Limits[corev1.ResourceName(re.Key)]; ok {
				if exactMatches[ctr.Name] == nil {
					exactMatches[ctr.Name] = make(map[string]map[string]string)
				}
				if regexpMatches[ctr.Name]["limits"] == nil {
					regexpMatches[ctr.Name]["limits"] = make(map[string]string)
				}
				exactMatches[ctr.Name]["limits"][re.Key] = v.String()
			}
		}
	} else {
		regexpMatches = make(map[string]map[string]map[string]string)
		for _, ctr := range append(podSpec.InitContainers, podSpec.Containers...) {
			for rn, rv := range ctr.Resources.Requests {
				if keyRE.MatchString(string(rn)) {
					if regexpMatches[ctr.Name] == nil {
						regexpMatches[ctr.Name] = make(map[string]map[string]string)
						regexpMatches[ctr.Name]["requests"] = make(map[string]string)
					}
					regexpMatches[ctr.Name]["requests"][string(rn)] = rv.String()
				}
			}
			for rn, rv := range ctr.Resources.Limits {
				if keyRE.MatchString(string(rn)) {
					if regexpMatches[ctr.Name] == nil {
						regexpMatches[ctr.Name] = make(map[string]map[string]string)
					}
					if regexpMatches[ctr.Name]["limits"] == nil {
						regexpMatches[ctr.Name]["limits"] = make(map[string]string)
					}
					regexpMatches[ctr.Name]["limits"][string(rn)] = rv.String()
				}
			}
		}
	}

	// analyze and validate matches

	var inconsistent bool
	var extractedVal string
	if exactMatches != nil {
		switch re.Condition {
		case collectorcontrollerv1alpha1.ResourceEntryConditionBothSameValue:
			var rval, lval string
			for _, resources := range exactMatches {
				// keep requests as first value, it has precedence
				rval_, rok := resources["requests"][re.Key]
				lval_, lok := resources["limits"][re.Key]
				if rok != lok || rval_ != lval_ {
					inconsistent = true
					break
				}

				if re.Value != "" {
					if re.Value != rval_ {
						// this is not a real match, so skip this container
						continue
					}
				} else if re.MatchValue != "" {
					if !valRE.MatchString(rval_) {
						// this is not a real match, so skip this container
						continue
					}
				}

				if re.ConsistentAcrossContainers {
					if rval == "" && rok {
						rval = rval_
					} else if rval != rval_ && rok {
						inconsistent = true
						break
					}
					if lval == "" && lok {
						lval = lval_
					} else if lval != lval_ && lok {
						inconsistent = true
						break
					}
				}
				if re.LabelValue == "" {
					if re.Value != "" {
						extractedVal = re.Value
					} else if re.MatchValue != "" {
						extractedVal = rval_ // use rval_ because eventually rval is not set
					}
					// we do not need to check for the case where both re.Value and re.MatchValue are empty
					// as it is a not valid configuration already validated
				} else if extractedVal == "" {
					extractedVal = re.LabelValue
				}
			}
		case collectorcontrollerv1alpha1.ResourceEntryConditionBothAnyValue:
			var rval, lval string
			for _, resources := range exactMatches {
				// keep requests as first value, it has precedence
				rval_, rok := resources["requests"][re.Key]
				lval_, lok := resources["limits"][re.Key]
				if rok != lok {
					inconsistent = true
					break
				}

				if re.Value != "" {
					if re.Value != rval_ && re.Value != lval_ {
						// this is not a real match, so skip this container
						continue
					}
				} else if re.MatchValue != "" {
					if !valRE.MatchString(rval_) && !valRE.MatchString(lval_) {
						// this is not a real match, so skip this container
						continue
					}
				}

				if re.ConsistentAcrossContainers {
					if rval == "" && rok {
						rval = rval_
					} else if rval != rval_ && rok {
						inconsistent = true
						break
					}
					if lval == "" && lok {
						lval = lval_
					} else if lval != lval_ && lok {
						inconsistent = true
						break
					}
				}
				if re.LabelValue == "" {
					if re.Value != "" {
						extractedVal = re.Value
					} else if re.MatchValue != "" {
						extractedVal = rval_ // use rval_ because eventually rval is not set
					} else if rok {
						// for *AnyValue condition and LabelValue, Value and MatchValue all empty
						// we have to pick the real resource value, prefferring requests
						extractedVal = rval_ // use rval_ because eventually rval is not set
					} else if lok {
						extractedVal = lval_
					}
				} else if extractedVal == "" {
					extractedVal = re.LabelValue
				}
				if !re.ConsistentAcrossContainers && extractedVal != "" {
					// we are done
					break
				}
			}
		case collectorcontrollerv1alpha1.ResourceEntryConditionAtLeastOneAnyValue:
			var rval, lval string
			for _, resources := range exactMatches {
				// keep requests as first value, it has precedence
				rval_, rok := resources["requests"][re.Key]
				lval_, lok := resources["limits"][re.Key]
				if !(rok || lok) {
					continue
				}
				if re.Value != "" {
					if re.Value != rval_ && re.Value != lval_ {
						// this is not a real match, so skip this container
						continue
					}
				} else if re.MatchValue != "" {
					if !valRE.MatchString(rval_) && !valRE.MatchString(lval_) {
						// this is not a real match, so skip this container
						continue
					}
				}

				if re.ConsistentAcrossContainers {
					if rval == "" {
						rval = rval_
					} else if rval != rval_ && rok {
						inconsistent = true
						break
					}
					if lval == "" {
						lval = lval_
					} else if lval != lval_ && lok {
						inconsistent = true
						break
					}
				}

				if re.LabelValue == "" {
					if re.Value != "" {
						extractedVal = re.Value
					} else if re.MatchValue != "" {
						if rok && valRE.MatchString(rval_) {
							extractedVal = rval_ // use rval_ because eventually rval is not set
						} else if lok && valRE.MatchString(lval_) {
							extractedVal = lval_
						}
					} else if rok {
						// for *AnyValue condition and LabelValue, Value and MatchValue all empty
						// we have to pick the real resource value, prefferring requests
						extractedVal = rval_ // use rval_ because eventually rval is not set
					} else if lok {
						extractedVal = lval_
					}
				} else if extractedVal == "" {
					extractedVal = re.LabelValue
				}
				if !re.ConsistentAcrossContainers && extractedVal != "" {
					// we are done
					break
				}
			}
		case collectorcontrollerv1alpha1.ResourceEntryConditionInRequestsAnyValue:
			var rval string
			for _, resources := range exactMatches {
				rval_, rok := resources["requests"][re.Key]
				if !rok {
					continue
				}
				if re.Value != "" {
					if re.Value != rval_ {
						// this is not a real match, so skip this container
						continue
					}
				} else if re.MatchValue != "" {
					if !valRE.MatchString(rval_) {
						// this is not a real match, so skip this container
						continue
					}
				}

				if re.ConsistentAcrossContainers {
					if rval == "" {
						rval = rval_
					} else if rval != rval_ && rok {
						inconsistent = true
						break
					}
				}

				if re.LabelValue == "" {
					if re.Value != "" {
						extractedVal = re.Value
					} else if re.MatchValue != "" {
						if rok && valRE.MatchString(rval_) {
							extractedVal = rval_ // use rval_ because eventually rval is not set
						}
					} else if rok {
						// for *AnyValue condition and LabelValue, Value and MatchValue all empty
						// we have to pick the real resource value, prefferring requests
						extractedVal = rval_ // use rval_ because eventually rval is not set
					}
				} else if extractedVal == "" {
					extractedVal = re.LabelValue
				}
				if !re.ConsistentAcrossContainers && extractedVal != "" {
					// we are done
					break
				}
			}
		case collectorcontrollerv1alpha1.ResourceEntryConditionInLimitsAnyValue:
			var lval string
			for _, resources := range exactMatches {
				lval_, lok := resources["limits"][re.Key]
				if lok {
					continue
				}
				if re.Value != "" {
					if re.Value != lval_ {
						// this is not a real match, so skip this container
						continue
					}
				} else if re.MatchValue != "" {
					if !valRE.MatchString(lval_) {
						// this is not a real match, so skip this container
						continue
					}
				}

				if re.ConsistentAcrossContainers {
					if lval == "" {
						lval = lval_
					} else if lval != lval_ && lok {
						inconsistent = true
						break
					}
				}

				if re.LabelValue == "" {
					if re.Value != "" {
						extractedVal = re.Value
					} else if re.MatchValue != "" {
						if lok && valRE.MatchString(lval_) {
							extractedVal = lval_
						}
					} else if lok {
						// for *AnyValue condition and LabelValue, Value and MatchValue all empty
						// we have to pick the real resource value, prefferring requests
						extractedVal = lval_
					}
				} else if extractedVal == "" {
					extractedVal = re.LabelValue
				}
				if !re.ConsistentAcrossContainers && extractedVal != "" {
					// we are done
					break
				}
			}
		}
	} else if regexpMatches != nil {
		switch re.Condition {
		case collectorcontrollerv1alpha1.ResourceEntryConditionBothSameValue:
			var rval, lval string
			for _, resources := range regexpMatches {
				// keep requests as first value, it has precedence
				var rval_, lval_, rn, ln string
				for rn_ := range resources["requests"] {
					rn = rn_
				}
				for ln_ := range resources["limits"] {
					ln = ln_
				}

				rval_, rok := resources["requests"][rn]
				lval_, lok := resources["limits"][ln]
				if rok != lok || rval_ != lval_ || rn != ln {
					inconsistent = true
					break
				}

				if re.Value != "" {
					if re.Value != rval_ {
						// this is not a real match, so skip this container
						continue
					}
				} else if re.MatchValue != "" {
					if !valRE.MatchString(rval_) {
						// this is not a real match, so skip this container
						continue
					}
				}

				if re.ConsistentAcrossContainers {
					if rval == "" && rok {
						rval = rval_
					} else if rval != rval_ && rok {
						inconsistent = true
						break
					}
					if lval == "" && lok {
						lval = lval_
					} else if lval != lval_ && lok {
						inconsistent = true
						break
					}
				}
				if re.LabelValue == "" {
					if re.Value != "" {
						extractedVal = re.Value
					} else if re.MatchValue != "" {
						extractedVal = rval_ // use rval_ because eventually rval is not set
					}
					// we do not need to check for the case where both re.Value and re.MatchValue are empty
					// as it is a not valid configuration already validated
				} else if extractedVal == "" {
					if re.MatchKey != "" && keyRE.NumSubexp() > 0 && (re.LabelValue[0] == '\\' || re.LabelValue[0] == '$') {
						idx, isDigitErr := strconv.Atoi(re.LabelValue[1:])
						if isDigitErr != nil {
							idx = -1
							if re.LabelValue[:2] == "${" && re.LabelValue[len(re.LabelValue)-1] == '}' {
								// assume this is a named group reference
								name := re.LabelValue[2 : len(re.LabelValue)-1]
								idx = keyRE.SubexpIndex(name)
							}
						}
						if idx == -1 {
							extractedVal = re.LabelValue
						} else {
							loc := keyRE.FindStringSubmatchIndex(rn)
							if loc == nil || len(loc) < idx*2 {
								// this is not a real match, so skip this container
								continue
							}
							extractedVal = rn[loc[idx*2]:loc[(idx*2)+1]]
						}
					} else {
						extractedVal = re.LabelValue
					}
				}
				if !re.ConsistentAcrossContainers && extractedVal != "" {
					// we are done
					break
				}
			}
		case collectorcontrollerv1alpha1.ResourceEntryConditionBothAnyValue:
			var rval, lval, rn, ln string
			for _, resources := range regexpMatches {
				for rn_ := range resources["requests"] {
					rn = rn_
				}
				for ln_ := range resources["limits"] {
					ln = ln_
				}
				// keep requests as first value, it has precedence
				rval_, rok := resources["requests"][rn]
				lval_, lok := resources["limits"][ln]
				if rok != lok {
					inconsistent = true
					break
				}

				if re.Value != "" {
					if re.Value != rval_ && re.Value != lval_ {
						// this is not a real match, so skip this container
						continue
					}
				} else if re.MatchValue != "" {
					if !valRE.MatchString(rval_) && !valRE.MatchString(lval_) {
						// this is not a real match, so skip this container
						continue
					}
				}

				if re.ConsistentAcrossContainers {
					if rval == "" && rok {
						rval = rval_
					} else if rval != rval_ && rok {
						inconsistent = true
						break
					}
					if lval == "" && lok {
						lval = lval_
					} else if lval != lval_ && lok {
						inconsistent = true
						break
					}
				}
				if re.LabelValue == "" {
					if re.Value != "" {
						extractedVal = re.Value
					} else if re.MatchValue != "" {
						extractedVal = rval_ // use rval_ because eventually rval is not set
					} else if rok {
						// for *AnyValue condition and LabelValue, Value and MatchValue all empty
						// we have to pick the real resource value, preferring requests
						extractedVal = rval_ // use rval_ because eventually rval is not set
					} else if lok {
						extractedVal = lval_
					}
				} else if extractedVal == "" {
					if re.MatchKey != "" && keyRE.NumSubexp() > 0 && (re.LabelValue[0] == '\\' || re.LabelValue[0] == '$') {
						idx, isDigitErr := strconv.Atoi(re.LabelValue[1:])
						if isDigitErr != nil {
							idx = -1
							if re.LabelValue[:2] == "${" && re.LabelValue[len(re.LabelValue)-1] == '}' {
								// assume this is a named group reference
								name := re.LabelValue[2 : len(re.LabelValue)-1]
								idx = keyRE.SubexpIndex(name)
							}
						}
						if idx == -1 {
							extractedVal = re.LabelValue
						} else {
							loc := keyRE.FindStringSubmatchIndex(rn)
							if loc == nil {
								// this is not a real match, so skip this container
								continue
							}
							extractedVal = rn[loc[0]:loc[1]]
						}
					} else {
						extractedVal = re.LabelValue
					}
				}
				if !re.ConsistentAcrossContainers && extractedVal != "" {
					// we are done
					break
				}
			}
		case collectorcontrollerv1alpha1.ResourceEntryConditionAtLeastOneAnyValue:
			var rval, lval, rn, ln string
			for _, resources := range regexpMatches {
				for rn_ := range resources["requests"] {
					rn = rn_
				}
				for ln_ := range resources["limits"] {
					ln = ln_
				}
				// keep requests as first value, it has precedence
				rval_, rok := resources["requests"][rn]
				lval_, lok := resources["limits"][ln]
				if !(rok || lok) {
					continue
				}
				if re.Value != "" {
					if re.Value != rval_ && re.Value != lval_ {
						// this is not a real match, so skip this container
						continue
					}
				} else if re.MatchValue != "" {
					if !valRE.MatchString(rval_) && !valRE.MatchString(lval_) {
						// this is not a real match, so skip this container
						continue
					}
				}

				if re.ConsistentAcrossContainers {
					if rval == "" {
						rval = rval_
					} else if rval != rval_ && rok {
						inconsistent = true
						break
					}
					if lval == "" {
						lval = lval_
					} else if lval != lval_ && lok {
						inconsistent = true
						break
					}
				}

				if re.LabelValue == "" {
					if re.Value != "" {
						extractedVal = re.Value
					} else if re.MatchValue != "" {
						if rok && valRE.MatchString(rval_) {
							extractedVal = rval_ // use rval_ because eventually rval is not set
						} else if lok && valRE.MatchString(lval_) {
							extractedVal = lval_
						}
					} else if rok {
						// for *AnyValue condition and LabelValue, Value and MatchValue all empty
						// we have to pick the real resource value, prefferring requests
						extractedVal = rval_ // use rval_ because eventually rval is not set
					} else if lok {
						extractedVal = lval_
					}
				} else if extractedVal == "" {
					if re.MatchKey != "" && keyRE.NumSubexp() > 0 && (re.LabelValue[0] == '\\' || re.LabelValue[0] == '$') {
						idx, isDigitErr := strconv.Atoi(re.LabelValue[1:])
						if isDigitErr != nil {
							idx = -1
							if re.LabelValue[:2] == "${" && re.LabelValue[len(re.LabelValue)-1] == '}' {
								// assume this is a named group reference
								name := re.LabelValue[2 : len(re.LabelValue)-1]
								idx = keyRE.SubexpIndex(name)
							}
						}
						if idx == -1 {
							extractedVal = re.LabelValue
						} else {
							extractFrom := rn
							if !rok && lok {
								extractFrom = ln
							}
							loc := keyRE.FindStringSubmatchIndex(extractFrom)
							if loc == nil {
								// this is not a real match, so skip this container
								continue
							}
							extractedVal = extractFrom[loc[0]:loc[1]]
						}
					} else {
						extractedVal = re.LabelValue
					}
				}
				if !re.ConsistentAcrossContainers && extractedVal != "" {
					// we are done
					break
				}
			}
		case collectorcontrollerv1alpha1.ResourceEntryConditionInRequestsAnyValue:
			var rval, rn string
			for _, resources := range regexpMatches {
				for rn_ := range resources["requests"] {
					rn = rn_
				}
				rval_, rok := resources["requests"][rn]
				if !rok {
					continue
				}
				if re.Value != "" {
					if re.Value != rval_ {
						// this is not a real match, so skip this container
						continue
					}
				} else if re.MatchValue != "" {
					if !valRE.MatchString(rval_) {
						// this is not a real match, so skip this container
						continue
					}
				}

				if re.ConsistentAcrossContainers {
					if rval == "" {
						rval = rval_
					} else if rval != rval_ && rok {
						inconsistent = true
						break
					}
				}

				if re.LabelValue == "" {
					if re.Value != "" {
						extractedVal = re.Value
					} else if re.MatchValue != "" {
						if rok && valRE.MatchString(rval_) {
							extractedVal = rval_ // use rval_ because eventually rval is not set
						}
					} else if rok {
						// for *AnyValue condition and LabelValue, Value and MatchValue all empty
						// we have to pick the real resource value, prefferring requests
						extractedVal = rval_ // use rval_ because eventually rval is not set
					}
				} else if extractedVal == "" {
					if re.MatchKey != "" && keyRE.NumSubexp() > 0 && (re.LabelValue[0] == '\\' || re.LabelValue[0] == '$') {
						idx, isDigitErr := strconv.Atoi(re.LabelValue[1:])
						if isDigitErr != nil {
							idx = -1
							if re.LabelValue[:2] == "${" && re.LabelValue[len(re.LabelValue)-1] == '}' {
								// assume this is a named group reference
								name := re.LabelValue[2 : len(re.LabelValue)-1]
								idx = keyRE.SubexpIndex(name)
							}
						}
						if idx == -1 {
							extractedVal = re.LabelValue
						} else {
							loc := keyRE.FindStringSubmatchIndex(rn)
							if loc == nil {
								// this is not a real match, so skip this container
								continue
							}
							extractedVal = rn[loc[0]:loc[1]]
						}
					} else {
						extractedVal = re.LabelValue
					}
				}
				if !re.ConsistentAcrossContainers && extractedVal != "" {
					// we are done
					break
				}
			}
		case collectorcontrollerv1alpha1.ResourceEntryConditionInLimitsAnyValue:
			var lval, ln string
			for _, resources := range regexpMatches {
				for ln_ := range resources["limits"] {
					ln = ln_
				}
				lval_, lok := resources["limits"][ln]
				if lok {
					continue
				}
				if re.Value != "" {
					if re.Value != lval_ {
						// this is not a real match, so skip this container
						continue
					}
				} else if re.MatchValue != "" {
					if !valRE.MatchString(lval_) {
						// this is not a real match, so skip this container
						continue
					}
				}

				if re.ConsistentAcrossContainers {
					if lval == "" {
						lval = lval_
					} else if lval != lval_ && lok {
						inconsistent = true
						break
					}
				}

				if re.LabelValue == "" {
					if re.Value != "" {
						extractedVal = re.Value
					} else if re.MatchValue != "" {
						if lok && valRE.MatchString(lval_) {
							extractedVal = lval_
						}
					} else if lok {
						// for *AnyValue condition and LabelValue, Value and MatchValue all empty
						// we have to pick the real resource value
						extractedVal = lval_
					}
				} else if extractedVal == "" {
					if re.MatchKey != "" && keyRE.NumSubexp() > 0 && (re.LabelValue[0] == '\\' || re.LabelValue[0] == '$') {
						idx, isDigitErr := strconv.Atoi(re.LabelValue[1:])
						if isDigitErr != nil {
							idx = -1
							if re.LabelValue[:2] == "${" && re.LabelValue[len(re.LabelValue)-1] == '}' {
								// assume this is a named group reference
								name := re.LabelValue[2 : len(re.LabelValue)-1]
								idx = keyRE.SubexpIndex(name)
							}
						}
						if idx == -1 {
							extractedVal = re.LabelValue
						} else {
							loc := keyRE.FindStringSubmatchIndex(ln)
							if loc == nil {
								// this is not a real match, so skip this container
								continue
							}
							extractedVal = ln[loc[0]:loc[1]]
						}
					} else {
						extractedVal = re.LabelValue
					}
				}
				if !re.ConsistentAcrossContainers && extractedVal != "" {
					// we are done
					break
				}
			}
		}
	} else {
		return re.LabelNegativeValue
	}
	if inconsistent {
		return re.LabelInconsistentValue
	}
	if extractedVal == "" && re.LabelNegativeValue != "" {
		return re.LabelNegativeValue
	}
	return extractedVal
}

// initExtensionLabelIndexes creates an id for each extension label and indexes it
// in the collector
func (c *Collector) initExtensionLabelIndexes() {
	c.labelIdsByNames = make(map[collectorcontrollerv1alpha1.LabelName]collectorcontrollerv1alpha1.LabelId)
	c.labelNamesByIds = make(map[collectorcontrollerv1alpha1.LabelId]collectorcontrollerv1alpha1.LabelName)
	c.labelsById = make(map[collectorcontrollerv1alpha1.LabelId]*collectorcontrollerv1alpha1.ExtensionLabel)
	c.taintLabelsById = make(map[collectorcontrollerv1alpha1.LabelId]*collectorcontrollerv1alpha1.NodeTaint)
	c.resourceEntryLabelsById = make(map[collectorcontrollerv1alpha1.LabelId]*collectorcontrollerv1alpha1.ResourceEntry)
	e := &c.Extensions

	// initialize each of the extension label ids (indexes)
	for i := range e.Pods {
		id := e.Pods[i].ID
		c.labelIdsByNames[e.Pods[i].LabelName] = id
		c.labelNamesByIds[id] = e.Pods[i].LabelName
		c.labelsById[id] = &e.Pods[i]
	}
	for i := range e.ResourceEntries {
		id := e.ResourceEntries[i].ID
		c.labelIdsByNames[e.ResourceEntries[i].LabelName] = id
		c.labelNamesByIds[id] = e.ResourceEntries[i].LabelName
		c.resourceEntryLabelsById[id] = &e.ResourceEntries[i]
	}
	for i := range e.Namespaces {
		id := e.Namespaces[i].ID
		c.labelIdsByNames[e.Namespaces[i].LabelName] = id
		c.labelNamesByIds[id] = e.Namespaces[i].LabelName
		c.labelsById[id] = &e.Namespaces[i]
	}
	for i := range e.Quota {
		id := e.Quota[i].ID
		c.labelIdsByNames[e.Quota[i].LabelName] = id
		c.labelNamesByIds[id] = e.Quota[i].LabelName
		c.labelsById[id] = &e.Quota[i]
	}
	for i := range e.Nodes {
		id := e.Nodes[i].ID
		c.labelIdsByNames[e.Nodes[i].LabelName] = id
		c.labelNamesByIds[id] = e.Nodes[i].LabelName
		c.labelsById[id] = &e.Nodes[i]
	}
	for i := range e.NodeTaints {
		id := e.NodeTaints[i].ID
		c.labelIdsByNames[e.NodeTaints[i].LabelName] = id
		c.labelNamesByIds[id] = e.NodeTaints[i].LabelName
		c.taintLabelsById[id] = &e.NodeTaints[i]
	}
	for i := range e.PVCs {
		id := e.PVCs[i].ID
		c.labelIdsByNames[e.PVCs[i].LabelName] = id
		c.labelNamesByIds[id] = e.PVCs[i].LabelName
		c.labelsById[id] = &e.PVCs[i]
	}
	for i := range e.PVs {
		id := e.PVs[i].ID
		c.labelIdsByNames[e.PVs[i].LabelName] = id
		c.labelNamesByIds[id] = e.PVs[i].LabelName
		c.labelsById[id] = &e.PVs[i]
	}

	for _, sc := range c.sideCarConfigs {
		for i := range sc.Labels {
			id := sc.Labels[i].ID
			c.labelIdsByNames[sc.Labels[i].LabelName] = id
			c.labelNamesByIds[id] = sc.Labels[i].LabelName
			c.labelsById[id] = &sc.Labels[i]
		}
	}
}
