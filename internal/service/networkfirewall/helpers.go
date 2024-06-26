// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package networkfirewall

import (
	"github.com/YakDriver/regexache"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/networkfirewall"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/hashicorp/terraform-provider-aws/names"
)

func encryptionConfigurationSchema() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeList,
		MaxItems: 1,
		Optional: true,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				names.AttrKeyID: {
					Type:     schema.TypeString,
					Optional: true,
				},
				names.AttrType: {
					Type:         schema.TypeString,
					Required:     true,
					ValidateFunc: validation.StringInSlice(networkfirewall.EncryptionType_Values(), false),
				},
			},
		},
	}
}

func expandEncryptionConfiguration(tfList []interface{}) *networkfirewall.EncryptionConfiguration {
	ec := &networkfirewall.EncryptionConfiguration{Type: aws.String(networkfirewall.EncryptionTypeAwsOwnedKmsKey)}
	if len(tfList) == 1 && tfList[0] != nil {
		tfMap := tfList[0].(map[string]interface{})
		if v, ok := tfMap[names.AttrKeyID].(string); ok {
			ec.KeyId = aws.String(v)
		}
		if v, ok := tfMap[names.AttrType].(string); ok {
			ec.Type = aws.String(v)
		}
	}

	return ec
}

func flattenEncryptionConfiguration(apiObject *networkfirewall.EncryptionConfiguration) []interface{} {
	if apiObject == nil || apiObject.Type == nil {
		return nil
	}
	if aws.StringValue(apiObject.Type) == networkfirewall.EncryptionTypeAwsOwnedKmsKey {
		return nil
	}

	m := map[string]interface{}{
		names.AttrKeyID: aws.StringValue(apiObject.KeyId),
		names.AttrType:  aws.StringValue(apiObject.Type),
	}

	return []interface{}{m}
}

func customActionSchema() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeSet,
		Optional: true,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"action_definition": {
					Type:     schema.TypeList,
					Required: true,
					MaxItems: 1,
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							"publish_metric_action": {
								Type:     schema.TypeList,
								Required: true,
								MaxItems: 1,
								Elem: &schema.Resource{
									Schema: map[string]*schema.Schema{
										"dimension": {
											Type:     schema.TypeSet,
											Required: true,
											Elem: &schema.Resource{
												Schema: map[string]*schema.Schema{
													names.AttrValue: {
														Type:     schema.TypeString,
														Required: true,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				"action_name": {
					Type:         schema.TypeString,
					Required:     true,
					ForceNew:     true,
					ValidateFunc: validation.StringMatch(regexache.MustCompile(`^[0-9A-Za-z]+$`), "must contain only alphanumeric characters"),
				},
			},
		},
	}
}

func expandCustomActions(l []interface{}) []*networkfirewall.CustomAction {
	if len(l) == 0 || l[0] == nil {
		return nil
	}

	customActions := make([]*networkfirewall.CustomAction, 0, len(l))
	for _, tfMapRaw := range l {
		customAction := &networkfirewall.CustomAction{}
		tfMap, ok := tfMapRaw.(map[string]interface{})
		if !ok {
			continue
		}
		if v, ok := tfMap["action_definition"].([]interface{}); ok && len(v) > 0 && v[0] != nil {
			customAction.ActionDefinition = expandActionDefinition(v)
		}
		if v, ok := tfMap["action_name"].(string); ok && v != "" {
			customAction.ActionName = aws.String(v)
		}
		customActions = append(customActions, customAction)
	}

	return customActions
}

func expandActionDefinition(l []interface{}) *networkfirewall.ActionDefinition {
	if l == nil || l[0] == nil {
		return nil
	}

	tfMap, ok := l[0].(map[string]interface{})
	if !ok {
		return nil
	}
	customAction := &networkfirewall.ActionDefinition{}

	if v, ok := tfMap["publish_metric_action"].([]interface{}); ok && len(v) > 0 && v[0] != nil {
		customAction.PublishMetricAction = expandCustomActionPublishMetricAction(v)
	}

	return customAction
}

func expandCustomActionPublishMetricAction(l []interface{}) *networkfirewall.PublishMetricAction {
	if len(l) == 0 || l[0] == nil {
		return nil
	}
	tfMap, ok := l[0].(map[string]interface{})
	if !ok {
		return nil
	}
	action := &networkfirewall.PublishMetricAction{}
	if tfSet, ok := tfMap["dimension"].(*schema.Set); ok && tfSet.Len() > 0 {
		tfList := tfSet.List()
		dimensions := make([]*networkfirewall.Dimension, 0, len(tfList))
		for _, tfMapRaw := range tfList {
			tfMap, ok := tfMapRaw.(map[string]interface{})
			if !ok {
				continue
			}
			dimension := &networkfirewall.Dimension{
				Value: aws.String(tfMap[names.AttrValue].(string)),
			}
			dimensions = append(dimensions, dimension)
		}
		action.Dimensions = dimensions
	}
	return action
}

func flattenCustomActions(c []*networkfirewall.CustomAction) []interface{} {
	if c == nil {
		return []interface{}{}
	}

	customActions := make([]interface{}, 0, len(c))
	for _, elem := range c {
		m := map[string]interface{}{
			"action_definition": flattenActionDefinition(elem.ActionDefinition),
			"action_name":       aws.StringValue(elem.ActionName),
		}
		customActions = append(customActions, m)
	}

	return customActions
}

func flattenActionDefinition(v *networkfirewall.ActionDefinition) []interface{} {
	if v == nil {
		return []interface{}{}
	}
	m := map[string]interface{}{
		"publish_metric_action": flattenPublishMetricAction(v.PublishMetricAction),
	}
	return []interface{}{m}
}

func flattenPublishMetricAction(m *networkfirewall.PublishMetricAction) []interface{} {
	if m == nil {
		return []interface{}{}
	}

	metrics := map[string]interface{}{
		"dimension": flattenDimensions(m.Dimensions),
	}

	return []interface{}{metrics}
}

func flattenDimensions(d []*networkfirewall.Dimension) []interface{} {
	dimensions := make([]interface{}, 0, len(d))
	for _, v := range d {
		dimension := map[string]interface{}{
			names.AttrValue: aws.StringValue(v.Value),
		}
		dimensions = append(dimensions, dimension)
	}

	return dimensions
}

func forceNewIfNotRuleOrderDefault(key string, d *schema.ResourceDiff) error {
	if d.Id() != "" && d.HasChange(key) {
		old, new := d.GetChange(key)
		defaultRuleOrderOld := old == nil || old.(string) == "" || old.(string) == networkfirewall.RuleOrderDefaultActionOrder
		defaultRuleOrderNew := new == nil || new.(string) == "" || new.(string) == networkfirewall.RuleOrderDefaultActionOrder

		if (defaultRuleOrderOld && !defaultRuleOrderNew) || (defaultRuleOrderNew && !defaultRuleOrderOld) {
			return d.ForceNew(key)
		}
	}
	return nil
}

func customActionSchemaDataSource() *schema.Schema {
	return &schema.Schema{
		Type:     schema.TypeSet,
		Computed: true,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"action_definition": {
					Type:     schema.TypeList,
					Computed: true,
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							"publish_metric_action": {
								Type:     schema.TypeList,
								Computed: true,
								Elem: &schema.Resource{
									Schema: map[string]*schema.Schema{
										"dimension": {
											Type:     schema.TypeSet,
											Computed: true,
											Elem: &schema.Resource{
												Schema: map[string]*schema.Schema{
													names.AttrValue: {
														Type:     schema.TypeString,
														Computed: true,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				"action_name": {
					Type:     schema.TypeString,
					Computed: true,
				},
			},
		},
	}
}
